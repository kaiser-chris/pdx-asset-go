package render

import (
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/model"
)

// The renderer's own shader, for a part the games' shaders cannot draw:
// one of an asset file read outside a game, whose shader files are not to
// be had, or one whose effect could not be built.
//
// It draws the material the way the games read it, but knows nothing of
// what the effect would do beyond that. What matters most to how a part
// looks it tells from the name of the effect its settings give, which the
// three games name alike: whether the palette colour is blended in, as into
// skin; whether what the diffuse map's alpha leaves out is cut away, as from
// leaves and hair; whether it is laid over what is behind it, as a decal is;
// and whether it is seen from both sides.

// fallbackLook is how the renderer's own shader draws a part.
type fallbackLook struct {
	// palette blends the palette colour in where the colour mask says,
	// cutout cuts away what the diffuse map's alpha leaves out, and blend
	// lays the part over what is behind it by that alpha, without hiding
	// what is drawn after it.
	palette, cutout, blend bool

	// noMetal leaves the metalness of the properties map out, as the
	// games' trees do, whose blue channel holds something else.
	noMetal bool

	// atlas reads the textures by the second set of texture coordinates,
	// as the games' effects of ATLAS do: Crusader Kings 3's and Europa
	// Universalis 5's buildings, which share an atlas of materials.
	atlas bool

	twoSided bool
}

// Words in the names of the games' effects, and what they say of how a part
// is drawn: portrait_skin, standard_alpha_to_coverage, tree_colormap,
// portrait_hair, decal_local, standard_alpha_blend, standard_two_sided.
var (
	paletteWords  = []string{"skin"}
	cutoutWords   = []string{"alpha_to_coverage", "tree", "hair", "foliage", "billboard"}
	blendWords    = []string{"decal", "alpha"}
	twoSidedWords = []string{"two_sided", "twosided", "hair", "foliage", "billboard"}
	noMetalWords  = []string{"tree", "foliage"}
	atlasWords    = []string{"atlas"}
)

// fallbackLookOf is how the renderer's own shader draws a part. A part
// without an effect, such as one of a bare mesh file, is drawn plain, with
// the palette colour, as a model of the viewer's own has always been.
func fallbackLookOf(part *model.Part) fallbackLook {
	name := strings.ToLower(part.Shader)
	if name == "" {
		return fallbackLook{palette: true}
	}

	look := fallbackLook{
		palette:  containsAny(name, paletteWords),
		cutout:   containsAny(name, cutoutWords),
		twoSided: containsAny(name, twoSidedWords),
		noMetal:  containsAny(name, noMetalWords),
		atlas:    containsAny(name, atlasWords),
	}

	// A cut away part is not blended as well: the effects of hair and
	// leaves cut by their alpha and write depth, and the alpha in their
	// names is that cut.
	look.blend = part.IsDecal() || !look.cutout && containsAny(name, blendWords)

	return look
}

func containsAny(name string, words []string) bool {
	for _, word := range words {
		if strings.Contains(name, word) {
			return true
		}
	}

	return false
}

// drawFallback draws the pieces of a part with the renderer's own shader.
func (m *Model) drawFallback(part *gpuPart, transform rl.Matrix) {
	r := m.renderer
	look := part.fallback

	// What raylib has batched is drawn first; the blending is set before
	// the shader is, for the reason drawEffect gives.
	rl.DrawRenderBatchActive()

	if look.blend {
		blendOver()
		rl.DisableDepthMask()
	}

	if look.twoSided {
		rl.DisableBackfaceCulling()
	}

	rl.SetShaderValue(r.shader, r.usePalette, []float32{boolFloat(look.palette)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.cutout, []float32{boolFloat(look.cutout)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.blended, []float32{boolFloat(look.blend)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.noMetal, []float32{boolFloat(look.noMetal)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.atlas, []float32{boolFloat(look.atlas)}, rl.ShaderUniformFloat)

	for _, piece := range part.pieces {
		rl.DrawMesh(*piece, *part.material, transform)
	}

	rl.DrawRenderBatchActive()

	if look.twoSided {
		rl.EnableBackfaceCulling()
	}

	if look.blend {
		rl.EnableDepthMask()
		rl.SetBlendMode(rl.BlendAlpha)
	}
}

func boolFloat(value bool) float32 {
	if value {
		return 1
	}

	return 0
}
