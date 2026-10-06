package render

import (
	"regexp"
	"strings"
)

// Textures that hold colours are sampled from sRGB.
//
// The engine converts a texture that holds colours from sRGB to linear
// light as the shaders sample it: the diffuse map, the further textures the
// mesh settings mark srgb = yes, and those the samplers mark sRGB = yes. The
// shaders light with what they sample as linear light. raylib makes no sRGB
// textures, so the conversion is added to the program instead: every place
// the pixel stage samples a texture goes through pdx_srgb, which converts
// what it sampled when the sampler's pdx_srgb_ switch is on. The switch is
// set as a texture is bound, since one effect reads colours through a slot
// for one asset and data for another.
//
// The conversion follows the filtering here rather than coming before it,
// as it does in hardware, which tells only at the edges of strong contrasts.

// samplingCall finds the start of a call that samples a texture.
var samplingCall = regexp.MustCompile(`\b(texture|textureLod|textureGrad|textureOffset|textureLodOffset|textureGradOffset|textureProj|textureProjLod|texelFetch|texelFetchOffset)\s*\(\s*(\w+)\s*,`)

// srgbSwitch is the name of the switch of a sampler.
func srgbSwitch(sampler string) string {
	return "pdx_srgb_" + sampler
}

// decodeSRGB has a pixel program convert what it samples through the given
// samplers from sRGB when their switch is on.
func decodeSRGB(glsl string, samplers []string) string {
	wanted := map[string]bool{}
	for _, name := range samplers {
		wanted[name] = true
	}

	var out strings.Builder

	rest := glsl
	used := map[string]bool{}

	for {
		match := samplingCall.FindStringSubmatchIndex(rest)
		if match == nil {
			out.WriteString(rest)

			break
		}

		name := rest[match[4]:match[5]]
		open := strings.IndexByte(rest[match[0]:], '(') + match[0]
		end := closingParen(rest, open)

		if !wanted[name] || end < 0 {
			out.WriteString(rest[:match[1]])
			rest = rest[match[1]:]

			continue
		}

		used[name] = true

		out.WriteString(rest[:match[0]])
		out.WriteString("pdx_srgb(")
		out.WriteString(rest[match[0] : end+1])
		out.WriteString(", " + srgbSwitch(name) + ")")
		rest = rest[end+1:]
	}

	if len(used) == 0 {
		return glsl
	}

	var declarations strings.Builder

	declarations.WriteString("\nvec4 pdx_srgb(vec4 color, bool on)\n{\n" +
		"    if (!on)\n    {\n        return color;\n    }\n" +
		"    vec3 low = color.rgb / 12.92;\n" +
		"    vec3 high = pow((color.rgb + 0.055) / 1.055, vec3(2.4));\n" +
		"    return vec4(mix(high, low, vec3(lessThanEqual(color.rgb, vec3(0.04045)))), color.a);\n}\n")

	for _, name := range samplers {
		if used[name] {
			declarations.WriteString("uniform bool " + srgbSwitch(name) + ";\n")
		}
	}

	// The declarations go after the version and extensions, before the
	// first function.
	program := out.String()

	at := strings.Index(program, "\nvoid ")
	if first := firstFunction.FindStringIndex(program); first != nil && (at < 0 || first[0] < at) {
		at = first[0]
	}

	if at < 0 {
		return glsl
	}

	return program[:at] + declarations.String() + program[at:]
}

// firstFunction finds the first function a program defines.
var firstFunction = regexp.MustCompile(`\n\w+\s+\w+\s*\([^;{]*\)\s*\n?\{`)

// closingParen finds the parenthesis that closes the one at open.
func closingParen(text string, open int) int {
	depth := 0

	for index := open; index < len(text); index++ {
		switch text[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}

	return -1
}
