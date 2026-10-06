package environment

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/folders"
)

type files map[string]string

func (f files) ReadFile(name string) ([]byte, error) {
	if text, ok := f[name]; ok {
		return []byte(text), nil
	}

	return nil, fmt.Errorf("%s, which none of the folders has", name)
}

const environmentFile = `
## Global environment settings
sun_color = { 1.02 0.98 0.96 }
sun_intensity = 5.0
sun_direction = { -0.5 0.55 0.9 }	#	+Right/-Left	Height  -Front/+Back
ambient_pos_x = hsv{ 0 1 0.5 }
cubemap_intensity = 2.5
cubemap = "gfx/map/environment/cubemap_blackbottom.dds"
cubemap_y_rotation = 260
fog_begin = 10.0
fog_color = hex{ 50779b }
map_objects_sunny_sun_azimuth = 0.25;
map_objects_sunny_sun_elevation = 0.5;
tree_sway_world_direction = { -1 0 -1 }
tonemap_function = "Uncharted"
tonemap_curve = {
	shoulder_strength = 0.318
}
`

func TestLoad(t *testing.T) {
	environment, err := Load(files{
		"paths.settings":      `gfx_environment_file = "gfx/environment.txt"`,
		"gfx/environment.txt": environmentFile,
		defaultFile:           "sun_intensity = 1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if environment.Path != "gfx/environment.txt" {
		t.Errorf("path = %s, want the one paths.settings names", environment.Path)
	}

	for name, want := range map[string][]float32{
		"SunDiffuse":             {1.02, 0.98, 0.96},
		"SunIntensity":           {5},
		"ToSunDir":               {-0.5, 0.55, 0.9},
		"AmbientPosX":            {0.5, 0, 0},
		"CubemapIntensity":       {2.5},
		"FogBegin2":              {10},
		"TreeSwayWorldDirection": {-1, 0, -1},
		"FogColor":               {0x50 / 255.0, 0x77 / 255.0, 0x9b / 255.0},
		// West, 45 degrees up.
		"ToMapObjectsSunnySunDir": {-float32(math.Sqrt2) / 2, float32(math.Sqrt2) / 2, 0},
	} {
		got := environment.Constants[name]
		if len(got) != len(want) {
			t.Errorf("%s = %v, want %v", name, got, want)

			continue
		}

		for index := range want {
			if math.Abs(float64(got[index]-want[index])) > 1e-5 {
				t.Errorf("%s = %v, want %v", name, got, want)
			}
		}
	}

	if environment.Cubemap != "gfx/map/environment/cubemap_blackbottom.dds" || environment.CubemapYRotation != 260 {
		t.Errorf("cubemap = %s turned %v", environment.Cubemap, environment.CubemapYRotation)
	}

	// Strings and blocks set no constants.
	for _, name := range []string{"TonemapFunction", "TonemapCurve", "Cubemap", "CubemapYRotation"} {
		if _, ok := environment.Constants[name]; ok {
			t.Errorf("%s is a constant", name)
		}
	}
}

func TestLoadWithoutPaths(t *testing.T) {
	environment, err := Load(files{defaultFile: "sun_intensity = 7"})
	if err != nil || environment.Constants["SunIntensity"][0] != 7 {
		t.Errorf("environment = %+v, %v, want the default file", environment, err)
	}

	fallback, err := Load(files{})
	if err == nil {
		t.Error("a game without an environment file loaded")
	}

	if fallback == nil || fallback.Constants["SunIntensity"][0] != 5 || fallback.Cubemap != "" {
		t.Errorf("environment without a file = %+v, want the defaults", fallback)
	}
}

// A file is read as it is: what it does not set is left out, rather than
// taken from the defaults, as Victoria 3's environment_greyscale.txt, which
// sets nothing but saturation_scale, is drawn by the game without the light
// of its environment file. Its post effect starts from a picture left as it
// is.
func TestReadLeavesOutWhatTheFileDoesNotSet(t *testing.T) {
	environment := Read("environment_greyscale.txt", "saturation_scale = 0.0")

	// The saturation, and the sun of the engine, which the file sets none of.
	if len(environment.Constants) != 4 || environment.Constants["SaturationScale"][0] != 0 || environment.Constants["SunIntensity"][0] != 1.5 {
		t.Errorf("constants = %v, want the saturation and the sun of the engine", environment.Constants)
	}

	if sun := Read("environment.txt", "sun_intensity = 5").Constants["SunIntensity"]; sun[0] != 5 {
		t.Errorf("a file's sun = %v, want its own", sun)
	}

	if environment.Cubemap != "" || environment.CubemapYRotation != 0 {
		t.Errorf("environment map %q turned %v, want none", environment.Cubemap, environment.CubemapYRotation)
	}

	post := environment.Post.Constants
	if hsv := post["HSV"]; hsv[0] != 0 || hsv[1] != 0 || hsv[2] != 1 {
		t.Errorf("HSV = %v, want the saturation of the file and nothing else changed", hsv)
	}

	if balance := post["ColorBalance"]; balance[0] != 1 || balance[1] != 1 || balance[2] != 1 {
		t.Errorf("colour balance = %v, want none", balance)
	}

	// The defaults themselves are not changed by what a file sets.
	if defaults["SunIntensity"][0] != 5 || Default().Post.Constants["HSV"][1] != 1 {
		t.Error("the defaults changed")
	}
}

func TestHSV(t *testing.T) {
	for _, test := range []struct {
		h, s, v float32
		rgb     [3]float32
	}{
		{0, 0, 1, [3]float32{1, 1, 1}},
		{0, 1, 1, [3]float32{1, 0, 0}},
		{1.0 / 3, 1, 1, [3]float32{0, 1, 0}},
		{2.0 / 3, 1, 0.5, [3]float32{0, 0, 0.5}},
		{0.5, 0, 1, [3]float32{1, 1, 1}},
	} {
		r, g, b := hsvToRGB(test.h, test.s, test.v)
		for index, got := range []float32{r, g, b} {
			if math.Abs(float64(got-test.rgb[index])) > 1e-5 {
				t.Errorf("hsv %v %v %v = %v %v %v, want %v", test.h, test.s, test.v, r, g, b, test.rgb)

				break
			}
		}
	}
}

// TestLoadInstalled reads the environment of the installations PDX_GAME_DIR
// names, through the files the engine reads it by.
func TestLoadInstalled(t *testing.T) {
	value := os.Getenv("PDX_GAME_DIR")
	if value == "" {
		t.Skip("set PDX_GAME_DIR to one or more game folders to run this test")
	}

	for _, game := range filepath.SplitList(value) {
		set := folders.Open([]folders.Source{{Name: "game", Path: game}})

		environment, err := Load(layeredSource{set})
		if err != nil {
			t.Errorf("%s: %v", game, err)

			continue
		}

		for _, name := range []string{"SunDiffuse", "SunIntensity", "ToSunDir", "CubemapIntensity"} {
			if len(environment.Constants[name]) == 0 {
				t.Errorf("%s: the environment sets no %s", game, name)
			}
		}

		if environment.Cubemap == "" {
			t.Errorf("%s: the environment names no cube map", game)
		}

		t.Logf("%s: %s, %d constants, cube map %s", filepath.Base(filepath.Dir(game)), environment.Path, len(environment.Constants), environment.Cubemap)
	}
}

// layeredSource reads a file from the folders, in the layers of Europa
// Universalis 5 the way the engine mounts them, the game's last.
type layeredSource struct {
	set *folders.Set
}

func (s layeredSource) ReadFile(name string) ([]byte, error) {
	for _, layer := range []string{"in_game/", "main_menu/", "loading_screen/", ""} {
		if found, ok := s.set.Find(layer + name); ok {
			return os.ReadFile(found.Path)
		}
	}

	return nil, fmt.Errorf("%s, which none of the folders has", name)
}

// The post effect's values come from the file, under the names of the
// members of its constant buffer, and its defaults where the file sets none.
func TestReadPost(t *testing.T) {
	environment := Read("environment.txt", `
exposure = 2.0
saturation_scale = 0.9
tonemap_function = "TonyMcMapface"
tonemap_whiteluminance = 3
tonemap_curve = {
	shoulder_strength = 0.5
}
`)

	post := environment.Post
	if post.Tonemap != "TonyMcMapface" || post.Exposure != "FixedExposure" {
		t.Errorf("tonemap %q, exposure %q", post.Tonemap, post.Exposure)
	}

	for name, want := range map[string][]float32{
		"FixedExposureValue":      {2},
		"HSV":                     {0, 0.9, 1},
		"TonemapShoulderStrength": {0.5},
		"TonemapLinearWhite":      {4.2},
		"LumWhite2":               {9},
	} {
		got := post.Constants[name]
		if len(got) != len(want) {
			t.Errorf("%s = %v, want %v", name, got, want)

			continue
		}

		for index := range want {
			if math.Abs(float64(got[index]-want[index])) > 1e-5 {
				t.Errorf("%s = %v, want %v", name, got, want)
			}
		}
	}

	if Default().Post.Tonemap != "Uncharted" {
		t.Errorf("default tonemap = %q", Default().Post.Tonemap)
	}
}

// The volume that applies everywhere is read over the environment file,
// with what it inherits, and the volume named standard where there is no
// default.
func TestLoadVolume(t *testing.T) {
	for _, test := range []struct {
		volumes, name string
		saturation    float32
	}{
		{`
posteffect_values = { name = base saturation_scale = 0.5 value_scale = 1.2 }
posteffect_values = { name = default inherit = base saturation_scale = 0.95 fog_max = 30 }
posteffect_values = { name = night inherit = default saturation_scale = 0.1 }
`, "default", 0.95},
		{`
posteffect_values = { name = standard saturation_scale = 0.7 }
posteffect_values = { name = cold inherit = standard saturation_scale = 0.2 }
`, "standard", 0.7},
	} {
		environment, err := LoadFileOnMap(files{
			defaultFile: "saturation_scale = 1.0 value_scale = 1.0",
			"gfx/map/post_effects/posteffect_volumes.txt": test.volumes,
		}, defaultFile)
		if err != nil {
			t.Fatal(err)
		}

		if environment.Volume != test.name {
			t.Errorf("volume = %q, want %q", environment.Volume, test.name)
		}

		if _, ok := environment.Constants["FogMax"]; ok {
			t.Errorf("%s: the fog of the volume was read", test.name)
		}

		if hsv := environment.Post.Constants["HSV"]; hsv[1] != test.saturation {
			t.Errorf("%s: saturation = %v, want %v", test.name, hsv[1], test.saturation)
		}
	}

	inherited, _ := LoadFileOnMap(files{
		defaultFile: "value_scale = 1.0",
		"gfx/map/post_effects/posteffect_volumes.txt": `
posteffect_values = { name = base value_scale = 1.2 }
posteffect_values = { name = default inherit = base }
`,
	}, defaultFile)

	if hsv := inherited.Post.Constants["HSV"]; hsv[2] != 1.2 {
		t.Errorf("value = %v, want the inherited 1.2", hsv[2])
	}
}

// The constants of the shaders are set from the graphics defines, by the
// name they share or by an alias, and from the defaults without a file.
func TestDefines(t *testing.T) {
	environment, err := Load(files{
		defaultFile: "sun_intensity = 1",
		"common/defines/00_graphics.txt": `
NGraphics = {
	MESHTINT_HEIGHT_MAX = 0.5
	SSAO_MESH_COLOR = { 0.1 0.2 0.3 0.0 }
}
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	for member, want := range map[string][]float32{
		"_MeshTintHeightMax": {0.5},
		"_SSAOColorMesh":     {0.1, 0.2, 0.3, 0},
		"_MeshTintColor":     {0.20, 0.14, 0.06, 1.0},
	} {
		got, ok := environment.Defines.Constant(member)
		if !ok || len(got) != len(want) || got[0] != want[0] {
			t.Errorf("%s = %v, want %v", member, got, want)
		}
	}

	if _, ok := environment.Defines.Constant("_NoSuchConstant"); ok {
		t.Error("a constant no define sets has a value")
	}

	if values, ok := Default().Defines.Constant("_DistanceRoughnessBlend"); !ok || values[0] != 80 {
		t.Errorf("default _DistanceRoughnessBlend = %v", values)
	}
}

// Any environment file of a game can be read in place of the one it lights
// its world with, and only environment files count as such.
func TestLoadFile(t *testing.T) {
	source := files{
		"paths.settings":             `gfx_environment_file = "gfx/map/environment/environment.txt"`,
		defaultFile:                  "sun_intensity = 5",
		"gfx/map/environment/ui.txt": "sun_intensity = 3",
	}

	if path := ConfiguredPath(source); path != defaultFile {
		t.Errorf("configured path = %s", path)
	}

	environment, err := LoadFile(source, "gfx/map/environment/ui.txt")
	if err != nil || environment.Constants["SunIntensity"][0] != 3 || environment.Path != "gfx/map/environment/ui.txt" {
		t.Errorf("environment = %+v, %v", environment, err)
	}

	if !IsEnvironment("cubemap = \"x.dds\"") || IsEnvironment("season_strength = 1") {
		t.Error("environment files were not told apart")
	}
}

// The defaults come with an environment map of their own, a face of one
// pixel each, in the order of the faces of a DDS cube map, turned as
// Victoria 3 turns the one it is made from.
func TestDefaultCube(t *testing.T) {
	cube := DefaultCube()

	if cube.Size != 1 || cube.Levels != 1 || len(cube.Data) != 6*4 {
		t.Fatalf("cube = %d across, %d levels, %d bytes; want six faces of one pixel", cube.Size, cube.Levels, len(cube.Data))
	}

	// The sun's side, +z, is the brightest, the bottom, -y, darker than the
	// top, +y.
	brightness := func(face int) int {
		return int(cube.Data[face*4]) + int(cube.Data[face*4+1]) + int(cube.Data[face*4+2])
	}

	for face := range 6 {
		if face != 4 && brightness(face) >= brightness(4) {
			t.Errorf("face %d is as bright as the sun's side", face)
		}
	}

	if brightness(3) >= brightness(2) {
		t.Error("the bottom is as bright as the top")
	}

	if Default().CubemapYRotation != 260 {
		t.Errorf("the defaults are turned %v, want Victoria 3's 260", Default().CubemapYRotation)
	}
}

// The model editor reads an environment without the post effect volume of
// the map: environment_greyscale.txt keeps its saturation of nothing.
func TestLoadFileLeavesOutTheVolume(t *testing.T) {
	source := files{
		"gfx/map/environment/environment_greyscale.txt": "saturation_scale = 0.0",
		"gfx/map/post_effects/posteffect_volumes.txt":   "posteffect_values = { name = default saturation_scale = 0.95 }",
	}

	environment, err := LoadFile(source, "gfx/map/environment/environment_greyscale.txt")
	if err != nil || environment.Volume != "" || environment.Post.Constants["HSV"][1] != 0 {
		t.Errorf("environment = %+v, %v; want the file's saturation without the volume", environment, err)
	}
}

// The shadows are cast from the sun offset by the file's offset, as
// Victoria 3's environment file casts them; a file that sets none of the
// shadows' keys has them cast from the sun itself, sampled a pixel around.
func TestReadShadow(t *testing.T) {
	environment := Read("environment.txt", `
sun_direction = { -0.5 0.55 0.9 }
shadow_direction_offset = { -0.48 0.0 -1.45 }
shadowmap_kernelscale = 2
shadowmap_depthbias = -0.002
`)

	if shadow := environment.Shadow; shadow.DirectionOffset != [3]float32{-0.48, 0, -1.45} || shadow.KernelScale != 2 || shadow.DepthBias != -0.002 {
		t.Errorf("shadow = %+v", shadow)
	}

	if _, ok := environment.Constants["ShadowDirectionOffset"]; ok {
		t.Error("the shadows' keys were taken for constants of the shaders")
	}

	// The sun of length one, -0.431 0.474 0.775, offset: -0.911 0.474
	// -0.675, made of length one.
	direction := environment.ShadowDirection()
	for axis, want := range [3]float64{-0.740, 0.384, -0.553} {
		if math.Abs(float64(direction[axis])-want) > 0.001 {
			t.Errorf("shadow direction = %v, want %.3v", direction, want)
		}
	}

	plain := Read("environment_greyscale.txt", "saturation_scale = 0.0")
	if plain.Shadow != engineShadow || plain.ShadowDirection() != plain.sunDirection() {
		t.Errorf("shadow of a file without = %+v from %v", plain.Shadow, plain.ShadowDirection())
	}

	if Default().Shadow.DirectionOffset[2] != -1.45 {
		t.Errorf("default shadow = %+v, want Victoria 3's", Default().Shadow)
	}

	// Without a sun at all, straight down.
	if (&Environment{}).ShadowDirection() != [3]float32{0, 1, 0} {
		t.Error("an environment without a sun casts no shadows straight down")
	}
}

// sunDirection is the direction to the sun of length one.
func (e *Environment) sunDirection() [3]float32 {
	sun := e.Constants["ToSunDir"]
	length := float32(math.Sqrt(float64(sun[0]*sun[0] + sun[1]*sun[1] + sun[2]*sun[2])))

	return [3]float32{sun[0] / length, sun[1] / length, sun[2] / length}
}
