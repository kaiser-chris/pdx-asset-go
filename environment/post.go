package environment

import (
	"maps"

	"github.com/kaiser-chris/pdx-parser-go/script"
)

// Post is what an environment file sets for the post effect that turns the
// lit picture into what the screen shows: Jomini's restorescene.shader,
// whose constant buffer, PdxConstantBuffer1, the engine fills from the file.
type Post struct {
	// Tonemap is the tonemap_function, such as Uncharted, and Exposure the
	// exposure_function, such as FixedExposure.
	Tonemap  string
	Exposure string

	// Constants are the values of the file by the member of the post
	// effect's constant buffer each sets.
	Constants map[string][]float32

	// LUT is the table of colour correction, below the game's root, or
	// empty.
	LUT string
}

// postDefaults are the post effect of Default, Victoria 3's environment
// file's: its tonemap, Uncharted, with its curve, a fixed exposure of 1, and
// hue, saturation, value, colour balance and levels left as they are.
var postDefaults = postEngine

// postEngine is what the post effect of a file starts from, for what the
// file does not set: a picture left as it is, but for the tonemap, which is
// Victoria 3's with its curve, the one tonemap all three games' files set
// the values of. Victoria 3's environment_greyscale.txt, which sets nothing
// but saturation_scale, is drawn by the game without its environment's
// light, so what a file leaves out the game does not take from elsewhere.
var postEngine = Post{
	Tonemap:  "Uncharted",
	Exposure: "FixedExposure",
	Constants: map[string][]float32{
		"FixedExposureValue":      {1},
		"HSV":                     {0, 1, 1},
		"ColorBalance":            {1, 1, 1},
		"LevelsMin":               {0, 0, 0},
		"LevelsMax":               {1, 1, 1},
		"TonemapShoulderStrength": {0.318},
		"TonemapLinearStrength":   {0.145},
		"TonemapLinearAngle":      {0.148},
		"TonemapToeStrength":      {0.423},
		"TonemapToeNumerator":     {0.025},
		"TonemapToeDenominator":   {0.288},
		"TonemapLinearWhite":      {4.2},
		"LumWhite2":               {1},
		"MiddleGrey":              {0.5},
		"Contrast":                {1},
		"Pivot":                   {0.18},
	},
}

// readPost reads the post effect's values out of the fields of an
// environment file and the constants already read from them.
//
// The members of the constant buffer are named after the keys of the file,
// as those of JominiEnvironment are, but for these: exposure sets
// FixedExposureValue; hue_offset, saturation_scale and value_scale make up
// HSV; colorbalance is ColorBalance; the fields of tonemap_curve set the
// members of the same name after Tonemap; tonemap_middlegrey is MiddleGrey;
// and tonemap_whiteluminance is the white point LumWhite2 is the square of.
func readPost(fields []script.Field, constants map[string][]float32) Post {
	post := Post{Tonemap: postEngine.Tonemap, Exposure: postEngine.Exposure, Constants: maps.Clone(postEngine.Constants)}

	for _, field := range fields {
		switch field.Key {
		case "tonemap_function":
			if text, ok := field.Value.Str(); ok && text != "" {
				post.Tonemap = text
			}
		case "exposure_function":
			if text, ok := field.Value.Str(); ok && text != "" {
				post.Exposure = text
			}
		case "lut":
			if text, ok := field.Value.Str(); ok {
				post.LUT = text
			}
		case "tonemap_curve":
			for _, curve := range field.Value.Fields {
				if values, ok := numbers(curve.Value); ok {
					post.Constants["Tonemap"+constantName(curve.Key)] = values
				}
			}
		}
	}

	set := func(name string, from string) {
		if values, ok := constants[from]; ok {
			post.Constants[name] = values
		}
	}

	set("FixedExposureValue", "Exposure")
	set("ColorBalance", "Colorbalance")
	set("LevelsMin", "LevelsMin")
	set("LevelsMax", "LevelsMax")
	set("MiddleGrey", "TonemapMiddlegrey")
	set("BrightThreshold", "BrightThreshold")
	set("Contrast", "Contrast")
	set("Pivot", "Pivot")

	hsv := append([]float32(nil), post.Constants["HSV"]...)
	for index, name := range []string{"HueOffset", "SaturationScale", "ValueScale"} {
		if values, ok := constants[name]; ok && len(values) == 1 {
			hsv[index] = values[0]
		}
	}
	post.Constants["HSV"] = hsv

	if white, ok := constants["TonemapWhiteluminance"]; ok && len(white) == 1 {
		post.Constants["LumWhite2"] = []float32{white[0] * white[0]}
	}

	return post
}
