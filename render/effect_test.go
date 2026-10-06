package render

import (
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/shader"
)

// The colour of a pixel program comes out opaque after the program's own
// main has written it, whatever alpha that wrote.
func TestOpaqueOutput(t *testing.T) {
	program := `#version 330

layout(location = 0) out vec4 _entryPointOutput;

void main()
{
    _entryPointOutput = vec4(1.0, 0.0, 0.0, 0.0);
}
`

	got := opaqueOutput(program)

	for _, want := range []string{
		"void pdx_main()\n{\n    _entryPointOutput = vec4(1.0, 0.0, 0.0, 0.0);",
		"void main()\n{\n    pdx_main();\n    _entryPointOutput.a = 1.0;\n}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("program lacks %q:\n%s", want, got)
		}
	}

	if strings.Count(got, "void main()") != 1 {
		t.Errorf("program has not exactly one main:\n%s", got)
	}

	// A program that writes no colour is left as it is.
	if depth := "#version 330\nvoid main()\n{\n}\n"; opaqueOutput(depth) != depth {
		t.Error("a program without a colour output was changed")
	}
}

// pixelProgram is a pixel program the way SPIRV-Cross writes one: the
// colour output at location 0, samplers combined with their sampler states,
// and a helper function before main, which reads a texture inside nested
// parentheses.
const pixelProgram = `#version 330

layout(location = 0) out vec4 _entryPointOutput_Color;
uniform sampler2D pdx_DiffuseMap_Texture_DiffuseMap_Sampler;
uniform sampler2D pdx_NormalMap_Texture_NormalMap_Sampler;
in vec2 pdx_varying_3;

vec4 shade(vec2 uv)
{
    return texture(pdx_DiffuseMap_Texture_DiffuseMap_Sampler, (uv * vec2(2.0)) + vec2(0.5));
}

void main()
{
    vec4 normal = textureLod(pdx_NormalMap_Texture_NormalMap_Sampler, pdx_varying_3, 0.0);
    _entryPointOutput_Color = shade(pdx_varying_3) * normal.x;
}
`

// What a sampler reads goes through pdx_srgb, behind a switch of its own,
// for the samplers named only, whole calls however nested; the declarations
// come before the first function.
func TestDecodeSRGB(t *testing.T) {
	got := decodeSRGB(pixelProgram, []string{"pdx_DiffuseMap_Texture_DiffuseMap_Sampler"})

	for _, want := range []string{
		"return pdx_srgb(texture(pdx_DiffuseMap_Texture_DiffuseMap_Sampler, (uv * vec2(2.0)) + vec2(0.5)), pdx_srgb_pdx_DiffuseMap_Texture_DiffuseMap_Sampler);",
		"vec4 normal = textureLod(pdx_NormalMap_Texture_NormalMap_Sampler, pdx_varying_3, 0.0);",
		"uniform bool pdx_srgb_pdx_DiffuseMap_Texture_DiffuseMap_Sampler;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("program lacks %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "pdx_srgb_pdx_NormalMap") {
		t.Errorf("the normal map, which was not named, got a switch:\n%s", got)
	}

	if declared, used := strings.Index(got, "vec4 pdx_srgb("), strings.Index(got, "vec4 shade("); declared < 0 || declared > used {
		t.Errorf("pdx_srgb is not declared before the first function:\n%s", got)
	}

	// The conversion is the exact sRGB curve, the one hardware applies.
	for _, want := range []string{"/ 12.92", "+ 0.055) / 1.055, vec3(2.4)", "0.04045"} {
		if !strings.Contains(got, want) {
			t.Errorf("the conversion lacks %q", want)
		}
	}

	if decodeSRGB(pixelProgram, nil) != pixelProgram {
		t.Error("a program without samplers to convert was changed")
	}
}

// An effect with alpha to coverage drops a pixel whose alpha covers less
// than half of it, after its own main has run.
func TestCoverageOutput(t *testing.T) {
	got := coverageOutput(pixelProgram)

	for _, want := range []string{
		"void pdx_coverage_main()",
		"pdx_coverage_main();\n    if (_entryPointOutput_Color.a < 0.5)\n    {\n        discard;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("program lacks %q:\n%s", want, got)
		}
	}

	// Made opaque after, as an effect without blending is, the cut is kept.
	both := opaqueOutput(got)
	if !strings.Contains(both, "void pdx_main()\n{\n    pdx_coverage_main();") || strings.Count(both, "void main()") != 1 {
		t.Errorf("coverage and opacity do not chain:\n%s", both)
	}
}

// The colour is marked magenta where any of its channels is not a number or
// infinite, by its bits, which a driver cannot take to be false.
func TestMarkNonFinite(t *testing.T) {
	got := markNonFinite(pixelProgram)

	for _, want := range []string{
		"pdx_marked_main();",
		"floatBitsToUint(_entryPointOutput_Color) & uvec4(0x7f800000u)",
		"_entryPointOutput_Color = vec4(1.0, 0.0, 1.0, 1.0);",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("program lacks %q:\n%s", want, got)
		}
	}
}

// The states of the files write their values in either case:
// tree.shader writes alphatocoverage, others AlphaToCoverage.
func TestStateIsSet(t *testing.T) {
	for values, want := range map[string]bool{
		"alphatocoverage=yes": true,
		"AlphaToCoverage=YES": true,
		"alphatocoverage=no":  false,
		"BlendEnable=yes":     false,
	} {
		key, value, _ := strings.Cut(values, "=")
		state := &shader.State{Values: map[string]string{key: value}}

		if got := stateIsSet(state, "AlphaToCoverage"); got != want {
			t.Errorf("%s: set = %v, want %v", values, got, want)
		}
	}
}

// The vertex stage of the post effect writes where in the picture each
// corner is to every input the game's pixel stage reads, under its name.
func TestPostVertex(t *testing.T) {
	got := postVertex(pixelProgram)

	for _, want := range []string{
		"in vec3 vertexPosition;",
		"in vec2 vertexTexCoord;",
		"out vec2 pdx_varying_3;",
		"pdx_varying_3 = vertexTexCoord;",
		"gl_Position = mvp * vec4(vertexPosition, 1.0);",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vertex stage lacks %q:\n%s", want, got)
		}
	}
}

// The tonemaps the environment files name, by the defines and numbers of
// Jomini's posteffect_base.fxh: Victoria 3 picks by define, Crusader Kings
// 3 by TonemapIndex.
func TestTonemaps(t *testing.T) {
	for name, want := range map[string]struct {
		define string
		index  int
	}{
		"Uncharted":        {"TONEMAP_UNCHARTED", 6},
		"TonyMcMapface":    {"TONEMAP_TONY_MCMAPFACE", 7},
		"ReinhardModified": {"TONEMAP_REINHARD_MODIFIED", 2},
		"FilmicACES_Hill":  {"TONEMAP_FILMICACES_HILL", 5},
	} {
		got, ok := tonemaps[strings.ToLower(name)]
		if !ok || got.define != want.define || got.index != want.index {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}

	if member("cb_PdxConstantBuffer1.TonemapIndex") != "TonemapIndex" || member("cb_1.Contrast") != "Contrast" {
		t.Error("members are not told from their buffers")
	}
}
