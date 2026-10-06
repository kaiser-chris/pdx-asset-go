package environment

import (
	"maps"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/script"
)

// The graphics defines.
//
// The constants the games' own shaders share, such as Victoria 3's
// GameSharedConstants, the engine fills from the block NGraphics of the
// defines: MESHTINT_COLOR sets _MeshTintColor, the colour the bottom of a
// building is tinted with, DISTANCE_ROUGHNESS_BLEND _DistanceRoughnessBlend.
// A member is named after its define, the words run together, with a few
// exceptions, which defineAliases lists.

// definesFiles are the files of the defines that hold NGraphics, in the
// order the games read them: Victoria 3 keeps it with the rest, Crusader
// Kings 3 and Europa Universalis 5 in a folder of their own.
var definesFiles = []string{
	"common/defines/00_graphics.txt",
	"common/defines/graphic/00_graphics.txt",
}

// graphicsBlock is the block of the defines the shaders' constants come from.
const graphicsBlock = "NGraphics"

// defineAliases are the members whose define is not named after them, by the
// member's name as normalName gives it, as the comments of the defines
// describe them.
var defineAliases = map[string]string{
	"ssaocolormesh":       "SSAO_MESH_COLOR",
	"ssaoalphatrees":      "SSAO_TREE_ALPHA",
	"ssaoalphaterrain":    "SSAO_TERRAIN_ALPHA",
	"nightlightcolor":     "NIGHT_LIGHT_COLOR_DEFAULT",
	"lightsfadetime":      "NIGHT_LIGHT_FADE_TIME",
	"lightsactivatebegin": "NIGHT_LIGHT_ACTIVATE_BEGIN",
	"lightsactivateend":   "NIGHT_LIGHT_ACTIVATE_END",
}

// defineDefaults are what the defines fall back on, Victoria 3's: those of
// the constants its models' shaders read.
var defineDefaults = map[string][]float32{
	"MESHTINT_HEIGHT_MIN":         {0},
	"MESHTINT_HEIGHT_MAX":         {0.07},
	"MESHTINT_COLOR":              {0.20, 0.14, 0.06, 1.0},
	"SSAO_TREE_ALPHA":             {0.75},
	"SSAO_TERRAIN_ALPHA":          {0.5},
	"SSAO_MESH_COLOR":             {0.20, 0.14, 0.06, 0},
	"DISTANCE_ROUGHNESS_POSITION": {1.8},
	"DISTANCE_ROUGHNESS_BLEND":    {80},
	"DISTANCE_ROUGHNESS_SCALE":    {0.5},
	"WATER_SHADOW_MULTIPLIER":     {0.75},
	"NIGHT_LIGHT_COLOR_DEFAULT":   {1.0, 0.16, 0.035, 6.5},
	"NIGHT_LIGHT_FADE_TIME":       {0.35},
	"NIGHT_LIGHT_ACTIVATE_BEGIN":  {0.3},
	"NIGHT_LIGHT_ACTIVATE_END":    {0.75},
}

// Defines are the numbers of NGraphics.
type Defines struct {
	// Values are the numbers by their define.
	Values map[string][]float32

	// byName are the defines by their name as normalName gives it.
	byName map[string]string
}

// newDefines makes defines of values.
func newDefines(values map[string][]float32) Defines {
	defines := Defines{Values: values, byName: map[string]string{}}
	for define := range values {
		defines.byName[normalName(define)] = define
	}

	return defines
}

// loadDefines reads NGraphics from the files of the defines the source has,
// over the defaults.
func loadDefines(source Source) Defines {
	values := maps.Clone(defineDefaults)

	for _, file := range definesFiles {
		data, err := source.ReadFile(file)
		if err != nil {
			continue
		}

		block, ok := script.Parse(string(data)).Get(graphicsBlock)
		if !ok {
			continue
		}

		for _, field := range block.Fields {
			if numbers, ok := numbers(field.Value); ok {
				values[field.Key] = numbers
			}
		}
	}

	return newDefines(values)
}

// Constant is the value of a constant of the shaders, by the member's name,
// such as _MeshTintColor, from the define it is set by.
func (d Defines) Constant(member string) ([]float32, bool) {
	name := normalName(member)

	define, ok := defineAliases[name]
	if !ok {
		define, ok = d.byName[name]
		if !ok {
			return nil, false
		}
	}

	values, ok := d.Values[define]

	return values, ok
}

// normalName is a name in lower case without its underscores, which a
// member and its define have in common: _MeshTintColor and MESHTINT_COLOR
// are both meshtintcolor.
func normalName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}
