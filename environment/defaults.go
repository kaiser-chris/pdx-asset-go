package environment

import (
	"maps"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// defaults are the constants of Default, the environment of a game whose
// environment file cannot be had: values the games' own files were read to
// hold, so that a model still comes out lit if a later version of a game
// moves or renames them. A file that can be had is read as it is: what it
// does not set, the game does not take from these, as Victoria 3's
// environment_greyscale.txt shows, which the game draws without light from
// an environment map and with a sun of its own.
//
// What all three games set is Victoria 3's, the middle of the three in how
// bright it lights; what only Crusader Kings 3 sets is Crusader Kings 3's.
// The constants of post processing, which the shaders of a model do not
// read, are left out.
var defaults = map[string][]float32{
	// Victoria 3, gfx/map/environment/environment.txt.
	"SunDiffuse":       {1.02, 0.98, 0.96},
	"SunIntensity":     {5},
	"ToSunDir":         {-0.5, 0.55, 0.9},
	"CubemapIntensity": {2.5},
	"FogColor":         {0.75, 0.75, 0.75},

	// Crusader Kings 3, gfx/map/environment/environment.txt: the suns of
	// its scenarios, from their azimuth and elevation, and the scales of the
	// light on its map objects.
	"ToTerrainSunnySunDir":         {-0.47552827, 0.58778524, 0.6545085},
	"ToTerrainOvercastSunDir":      {-0.29389262, 0.309017, 0.9045085},
	"ToMapObjectsSunnySunDir":      {-0.76942086, 0.58778524, -0.25000006},
	"ToMapObjectsOvercastSunDir":   {-0.9393474, 0.15643446, -0.30521256},
	"ToWaterSunnySunDir":           {-0.47552827, 0.58778524, 0.6545085},
	"ToWaterOvercastSunDir":        {-0.47552827, 0.58778524, 0.6545085},
	"MapObjectsDiffuseLightScale":  {1},
	"MapObjectsSpecularLightScale": {1},
	"MapObjectsDiffuseIblScale":    {1},
	"MapObjectsSpecularIblScale":   {1},
}

// Default is the environment of no file: the defaults alone, lit by
// DefaultCube, turned as Victoria 3 turns the environment map it is made
// from.
func Default() *Environment {
	return &Environment{
		Constants:        defaultConstants(),
		Post:             Post{Tonemap: postDefaults.Tonemap, Exposure: postDefaults.Exposure, Constants: maps.Clone(postDefaults.Constants)},
		Defines:          newDefines(maps.Clone(defineDefaults)),
		CubemapYRotation: defaultCubemapYRotation,
		Shadow:           defaultShadow,
	}
}

// defaultShadow are the shadows of Victoria 3's environment file: cast
// longer and from further to the side than the sun lights.
var defaultShadow = Shadow{DirectionOffset: [3]float32{-0.48, 0, -1.45}, KernelScale: 1}

// defaultCubemapYRotation is how far Victoria 3 turns its environment map,
// cubemap_y_rotation of its environment file.
const defaultCubemapYRotation = 260

// defaultCubeFaces are the average colours of the faces of Victoria 3's
// environment map, gfx/map/environment/cubemap_blackbottom.dds, as it stores
// them, in the order of its faces: +x, -x, +y, -y, +z, -z. The sky around,
// the sun's side brighter, and the black bottom it is named after.
var defaultCubeFaces = [6][3]byte{
	{120, 127, 134},
	{121, 127, 134},
	{100, 111, 123},
	{54, 56, 58},
	{186, 195, 203},
	{50, 55, 60},
}

// DefaultCube is the environment map of the defaults, for an environment
// whose own cannot be had: a face of one pixel of the average colour of each
// face of Victoria 3's. The light it gives what it surrounds is what the
// environment map gives, which the shaders read at its smallest levels, the
// averages of its faces; what is reflected in a mirror is plain.
func DefaultCube() *texture.Cube {
	cube := &texture.Cube{Size: 1, Format: texture.RGBA8, Levels: 1}

	for _, face := range defaultCubeFaces {
		cube.Data = append(cube.Data, face[0], face[1], face[2], 255)
	}

	return cube
}

func defaultConstants() map[string][]float32 {
	constants := make(map[string][]float32, len(defaults))
	for name, values := range maps.All(defaults) {
		constants[name] = append([]float32(nil), values...)
	}

	return constants
}
