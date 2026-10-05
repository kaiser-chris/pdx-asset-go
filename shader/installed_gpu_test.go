//go:build uitest

package shader

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"
)

// defaultShaderFile is the shader file of mesh settings that name an effect
// but no file. Which file the engine takes then is not established; the
// effects such settings name are all of pdxmesh.shader.
const defaultShaderFile = "gfx/FX/pdxmesh.shader"

// use is one effect as the assets use it: the file, the effect, the defines
// the mesh settings add, and the layer of the asset.
type use struct {
	file, effect, defines, layer string
}

// TestCompileInstalledEffects builds every effect the assets of the
// installations PDX_GAME_DIR names use, compiles it from HLSL to GLSL, and has
// the OpenGL driver compile and link the GLSL in a hidden window. Every one of
// them has to: the shipped effects of Victoria 3, Europa Universalis 5 and
// Crusader Kings 3 all do. With PDX_DUMP_DIR set, the HLSL, the GLSL and the
// log of every program that does not are written out.
func TestCompileInstalledEffects(t *testing.T) {
	value := os.Getenv("PDX_GAME_DIR")
	if value == "" {
		t.Skip("set PDX_GAME_DIR to one or more game folders to run this test")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rl.SetTraceLogLevel(rl.LogWarning)
	rl.SetConfigFlags(rl.FlagWindowHidden)
	rl.InitWindow(64, 64, "shader tests")
	defer rl.CloseWindow()

	t.Logf("OpenGL: %s", glVersion())

	compiler, err := newGLCompiler()
	if err != nil {
		t.Fatal(err)
	}

	// Not subtests: a subtest runs on a goroutine of its own, and the OpenGL
	// context belongs to the thread this one is locked to.
	for _, game := range filepath.SplitList(value) {
		measureGame(t, game, compiler)
	}
}

// measureGame compiles the effects of one game and logs the outcome.
func measureGame(t *testing.T, game string, compiler *glCompiler) {
	name := filepath.Base(filepath.Dir(filepath.Clean(game)))
	dump := os.Getenv("PDX_DUMP_DIR")

	set := folders.Open(append(Engine(game), folders.Source{Name: "game", Path: game}))
	uses := effectsOf(set, asset.Load(set))

	if len(uses) == 0 {
		t.Errorf("%s: the assets use no effects", name)

		return
	}

	libraries := map[string]*Library{}
	reasons := map[string][]string{}
	counts := map[string]int{}

	note := func(reason, label string) {
		reasons[reason] = append(reasons[reason], label)
	}

	for _, used := range uses {
		library, ok := libraries[used.layer]
		if !ok {
			library = NewLibrary(Folders{Set: set, Layer: used.layer})
			libraries[used.layer] = library
		}

		label := fmt.Sprintf("%s :: %s %s", used.file, used.effect, used.defines)

		// The engine sets PDX_MESH_UV1 for a mesh with a second set of
		// texture coordinates, which the shaders of flags read; the models
		// of this module always have one.
		defines := []string{"PDX_MESH_UV1"}
		if used.defines != "" {
			defines = append(defines, strings.Split(used.defines, " | ")...)
		}

		write := func(program *Program, log string) {
			if dump == "" || program == nil {
				return
			}

			base := filepath.Join(dump, name, sanitize(label))
			_ = os.MkdirAll(filepath.Dir(base), 0o755)
			_ = os.WriteFile(base+".vert.hlsl", []byte(program.VertexHLSL), 0o644)
			_ = os.WriteFile(base+".frag.hlsl", []byte(program.PixelHLSL), 0o644)
			_ = os.WriteFile(base+".vert", []byte(program.Vertex), 0o644)
			_ = os.WriteFile(base+".frag", []byte(program.Pixel), 0o644)
			_ = os.WriteFile(base+".log", []byte(log), 0o644)
		}

		program, err := library.Program(used.file, used.effect, defines)

		var stageErr *StageError

		switch {
		case errors.As(err, &stageErr):
			counts["not cross compiled"]++
			note(fmt.Sprintf("HLSL, %s stage, %s: %s", stageErr.Stage, stageErr.Step, firstError(stageErr.Log)), label)
			write(program, stageErr.Log)

			continue

		case err != nil:
			counts["not built"]++
			note("not built: "+firstLine(err.Error()), label)

			continue
		}

		failed, log := compiler.compile(program.Vertex, program.Pixel)
		if failed == "" {
			counts["compiled"]++

			continue
		}

		counts["not compiled by the driver"]++
		note("GLSL, "+failed+" stage: "+firstError(log), label)
		write(program, log)
	}

	t.Logf("%s: %d effects used: %d compiled; %d not cross compiled, %d not compiled by the driver, %d not built",
		name, len(uses), counts["compiled"], counts["not cross compiled"], counts["not compiled by the driver"], counts["not built"])

	keys := slices.Collect(maps.Keys(reasons))
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(reasons[b]), len(reasons[a])), cmp.Compare(a, b))
	})

	for _, reason := range keys {
		t.Errorf("%s: %3d  %s\n       e.g. %s", name, len(reasons[reason]), reason, reasons[reason][0])
	}
}

// effectsOf lists the effects the mesh settings of the assets use, each
// once.
func effectsOf(set *folders.Set, assets *asset.Assets) []use {
	layers := set.Layers()
	seen := map[use]bool{}

	var uses []use

	add := func(origin string, settings []asset.MeshSettings) {
		layer := ""
		if first, _, found := strings.Cut(origin, "/"); found && slices.Contains(layers, first) {
			layer = first
		}

		for _, setting := range settings {
			if setting.Shader == "" {
				continue
			}

			file := cmp.Or(setting.ShaderFile, defaultShaderFile)
			defines := slices.Clone(setting.ShaderDefines)
			slices.Sort(defines)

			used := use{file: file, effect: setting.Shader, defines: strings.Join(defines, " | "), layer: layer}
			if !seen[used] {
				seen[used] = true
				uses = append(uses, used)
			}
		}
	}

	for _, mesh := range assets.Meshes.All() {
		add(mesh.Origin().File, mesh.Settings)
	}

	for _, entity := range assets.Entities.All() {
		add(entity.Origin().File, entity.Settings)
	}

	return uses
}

// errorLine finds the message in what a driver reports for an error, in the
// forms NVIDIA, AMD and Mesa write it.
var errorLine = regexp.MustCompile(`\d+\(\d+\) : error \w+: (.*)|ERROR: \d+:\d+: (.*)|\d+:\d+\(\d+\): error: (.*)|error: (.*)`)

// firstError picks the first error out of a driver's log, without the line
// it was on, so that errors of one kind count together.
func firstError(log string) string {
	if match := errorLine.FindStringSubmatch(log); match != nil {
		return firstLine(cmp.Or(match[1], match[2], match[3], match[4]))
	}

	return firstLine(log)
}

func sanitize(label string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', ' ', '*', '?', '"', '<', '>', '|':
			return '_'
		}

		return r
	}, label)
}

func glVersion() string {
	switch rl.GetVersion() {
	case rl.Opengl33:
		return "3.3"
	case rl.Opengl43:
		return "4.3"
	}

	return fmt.Sprint(rl.GetVersion())
}
