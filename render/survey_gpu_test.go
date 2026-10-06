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
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// TestSurvey draws every SURVEY_STEP-th entity of the game SURVEY_GAME
// names, and writes each picture to SURVEY_OUT along with a list of those
// that came out mostly black, mostly white or empty.
//
// SURVEY_FILTER keeps the entities whose name holds it, SURVEY_ENTITY one
// entity alone. SURVEY_SIZE is the size of the pictures, and SURVEY_YAW and
// SURVEY_PITCH turn the camera, in radians.
func TestSurvey(t *testing.T) {
	game := os.Getenv("SURVEY_GAME")
	if game == "" {
		t.Skip("set SURVEY_GAME to a game folder to survey its entities")
	}

	out := os.Getenv("SURVEY_OUT")
	step, _ := strconv.Atoi(os.Getenv("SURVEY_STEP"))
	step = max(step, 1)
	filter := os.Getenv("SURVEY_FILTER")
	only := os.Getenv("SURVEY_ENTITY")

	set := folders.Open(append(entity.EngineFolders(game), folders.Source{Name: "game", Path: game}))
	assets := asset.Load(set)
	loader := entity.NewLoader(set, assets)

	renderer := withRenderer(t)

	var names []string
	for name := range assets.Entities.All() {
		if _, ok := assets.MeshOf(name); ok && strings.Contains(name, filter) && (only == "" || name == only) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	size, _ := strconv.Atoi(os.Getenv("SURVEY_SIZE"))
	size = max(size, 128)

	pitch, yaw := 0.3, 0.6
	if value, err := strconv.ParseFloat(os.Getenv("SURVEY_PITCH"), 64); err == nil {
		pitch = value
	}

	if value, err := strconv.ParseFloat(os.Getenv("SURVEY_YAW"), 64); err == nil {
		yaw = value
	}

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

		uploaded, err := renderer.Upload(built)
		if err != nil {
			loader.Forget()

			continue
		}

		viewer := renderer.NewViewer(int32(size), int32(size))
		viewer.Frame(uploaded.Min, uploaded.Max)
		viewer.Rotate(yaw, pitch)

		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()

		picture := viewer.Image()
		verdict := judge(picture)
		counts[verdict]++

		shaders := map[string]bool{}
		for _, part := range built.Parts {
			shaders[part.Shader] = true
		}

		list := slices.Sorted(func(yield func(string) bool) {
			for shader := range shaders {
				if !yield(shader) {
					return
				}
			}
		})

		report = append(report, fmt.Sprintf("%s\t%s\t%s", verdict, name, strings.Join(list, ",")))

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

// judge says how a picture came out: empty, black or white for most of the
// model in one of those, ok otherwise.
func judge(picture *image.RGBA) string {
	var drawn, black, white int

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
		}
	}

	switch {
	case drawn < 20:
		return "empty"
	case black*10 > drawn*6:
		return "black"
	case white*10 > drawn*6:
		return "white"
	}

	return "ok"
}
