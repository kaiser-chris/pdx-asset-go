//go:build uitest

package render

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/entity"
	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/shader"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// TestSurvey draws every SURVEY_STEP-th entity of the game SURVEY_GAME
// names with the game's own effects, and writes each picture to SURVEY_OUT
// along with a list of those that came out mostly black, mostly white or
// empty: what defaults are missing shows there.
func TestSurvey(t *testing.T) {
	game := os.Getenv("SURVEY_GAME")
	if game == "" {
		t.Skip("set SURVEY_GAME to a game folder to survey its entities")
	}

	out := os.Getenv("SURVEY_OUT")
	step, _ := strconv.Atoi(os.Getenv("SURVEY_STEP"))
	step = max(step, 1)
	filter := os.Getenv("SURVEY_FILTER")

	set := folders.Open(append(shader.Engine(game), folders.Source{Name: "game", Path: game}))
	assets := asset.Load(set)
	loader := entity.NewLoader(set, assets)

	renderer := withRenderer(t)
	renderer.markNonFinite = true
	source := shader.Folders{Set: set, Layer: os.Getenv("SURVEY_LAYER")}
	// SURVEY_PATCH_FILE, _OLD and _NEW replace text in one shader file, to
	// see what a step of it computes.
	if file := os.Getenv("SURVEY_PATCH_FILE"); file != "" {
		renderer.UseShaders(patchedSource{Source: source, file: file, old: os.Getenv("SURVEY_PATCH_OLD"), new: os.Getenv("SURVEY_PATCH_NEW")})
	} else if os.Getenv("SURVEY_OWN_SHADER") == "" {
		renderer.UseShaders(source)
	}

	lighting, _ := environment.Load(source)
	if file := os.Getenv("SURVEY_ENV"); file != "" {
		lighting, _ = environment.LoadFile(source, file)
	}
	if os.Getenv("SURVEY_NO_VOLUME") != "" {
		lighting.Post = environment.Default().Post
		lighting.Post.Constants["HSV"] = []float32{0, 1, 1}
		lighting.Post.Constants["ColorBalance"] = []float32{1, 1, 1}
		lighting.Post.Constants["LevelsMin"] = []float32{0, 0, 0}
		lighting.Post.Constants["LevelsMax"] = []float32{1, 1, 1}
	}

	if os.Getenv("SURVEY_NO_DEFINES") != "" {
		lighting.Defines = environment.Defines{}
	}

	var cube *texture.Cube
	if data, err := source.ReadFile(lighting.Cubemap); lighting.Cubemap != "" && err == nil {
		cube, _ = texture.DecodeCube(data)
	}

	if sun := os.Getenv("SURVEY_SUN"); sun != "" {
		var x, y, z, intensity float32
		fmt.Sscanf(sun, "%f %f %f %f", &x, &y, &z, &intensity)
		lighting.Constants["ToSunDir"] = []float32{x, y, z}
		lighting.Constants["SunIntensity"] = []float32{intensity}
		lighting.Constants["SunDiffuse"] = []float32{1, 1, 1}
	}

	if os.Getenv("SURVEY_BUILTIN") != "" {
		lighting, cube = environment.Default(), environment.DefaultCube()
	}

	if err := renderer.SetEnvironment(lighting, cube); err != nil {
		t.Fatal(err)
	}

	var names []string
	for name := range assets.Entities.All() {
		if _, ok := assets.MeshOf(name); ok && strings.Contains(name, filter) && (os.Getenv("SURVEY_ENTITY") == "" || name == os.Getenv("SURVEY_ENTITY")) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var report []string
	counts := map[string]int{}

	for index := 0; index < len(names); index += step {
		name := names[index]

		built, _, err := loader.Load(name)
		if err != nil {
			continue
		}

		built.Parts = slices.DeleteFunc(built.Parts, func(part model.Part) bool { return part.Shader == "" })
		if len(built.Parts) == 0 {
			loader.Forget()

			continue
		}
		built.Bounds()

		if os.Getenv("SURVEY_UNITS") != "" {
			for index := range built.Parts {
				_, err := renderer.effectNamed(&built.Parts[index], built.Parts[index].ShadowShader, false)
				t.Logf("shadow effect %q: %v", built.Parts[index].ShadowShader, err)
			}
		}

		uploaded, err := renderer.Upload(built)
		if err != nil {
			loader.Forget()

			continue
		}

		if os.Getenv("SURVEY_UNITS") != "" {
			for _, part := range uploaded.parts {
				if part.effect == nil {
					continue
				}

				t.Logf("casts a shadow: %v; shadow constants %v; camera %v", part.shadow != nil, part.effect.shadow, part.effect.camera)

				for member := range part.effect.shared {
					if values, ok := lighting.Defines.Constant(member); ok {
						t.Logf("define sets %s = %v", member, values)
					}
				}

				for _, bound := range part.effect.textures {
					ref := ""
					if bound.texture.Sampler != nil {
						ref = bound.texture.Sampler.Ref
					}

					t.Logf("unit %d: %s %s ref=%q", bound.unit, bound.texture.Type, bound.texture.Name, ref)
				}
			}
		}

		size, _ := strconv.Atoi(os.Getenv("SURVEY_SIZE"))
		size = max(size, 128)

		viewer := renderer.NewViewer(int32(size), int32(size))
		viewer.Frame(uploaded.Min, uploaded.Max)
		viewer.Shadows = os.Getenv("SURVEY_NO_SHADOWS") == ""
		pitch := 0.3
		if value, err := strconv.ParseFloat(os.Getenv("SURVEY_PITCH"), 64); err == nil {
			pitch = value
		}

		yaw := 0.6
		if value, err := strconv.ParseFloat(os.Getenv("SURVEY_YAW"), 64); err == nil {
			yaw = value
		}

		viewer.Rotate(yaw, pitch)

		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()

		if err := viewer.PostError(); err != nil {
			t.Logf("post effect: %v", err)
		}

		picture := viewer.Image()
		verdict := judge(picture)
		counts[verdict]++

		shaders := map[string]bool{}
		for _, part := range built.Parts {
			shaders[part.Shader] = true
		}
		var list []string
		for shader := range shaders {
			list = append(list, shader)
		}
		sort.Strings(list)

		report = append(report, fmt.Sprintf("%s\t%s\t%s\t%d problems", verdict, name, strings.Join(list, ","), len(uploaded.Problems)))
		for _, problem := range uploaded.Problems {
			report = append(report, "\t\t"+firstLine(problem))
		}

		if out != "" {
			if file, err := os.Create(filepath.Join(out, verdict+"_"+name+".png")); err == nil {
				_ = png.Encode(file, picture)
				file.Close()
			}
		}

		viewer.Unload()
		uploaded.Unload()
		loader.Forget()
	}

	t.Logf("%v", counts)

	if out != "" {
		_ = os.WriteFile(filepath.Join(out, "survey.txt"), []byte(strings.Join(report, "\n")+"\n"), 0o644)
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")

	return line
}

// judge says how a picture came out: empty, black or white for most of the
// model in one of those, ok otherwise.
func judge(picture *image.RGBA) string {
	var drawn, black, white, magenta int

	for y := picture.Rect.Min.Y; y < picture.Rect.Max.Y; y++ {
		for x := picture.Rect.Min.X; x < picture.Rect.Max.X; x++ {
			pixel := picture.RGBAAt(x, y)
			if pixel.A < 128 {
				continue
			}

			drawn++

			if pixel.R < 8 && pixel.G < 8 && pixel.B < 8 {
				black++
			}

			if pixel.R > 247 && pixel.G > 247 && pixel.B > 247 {
				white++
			}

			if pixel.R > 247 && pixel.G < 8 && pixel.B > 247 {
				magenta++
			}
		}
	}

	switch {
	case drawn < 20:
		return "empty"
	case magenta*10 > drawn:
		return "nan"
	case black*10 > drawn*6:
		return "black"
	case white*10 > drawn*6:
		return "white"
	}

	return "ok"
}

// patchedSource reads shader files, with text replaced in one of them.
type patchedSource struct {
	shader.Source
	file, old, new string
}

func (p patchedSource) ReadFile(name string) ([]byte, error) {
	data, err := p.Source.ReadFile(name)
	if err == nil && name == p.file {
		if !strings.Contains(string(data), p.old) {
			return nil, fmt.Errorf("%s does not hold the text to patch", name)
		}

		data = []byte(strings.Replace(string(data), p.old, p.new, 1))
	}

	return data, err
}
