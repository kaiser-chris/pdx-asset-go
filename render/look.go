package render

import (
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/pattern"
)

// How a part is drawn.
//
// The renderer draws every part with a shader of its own, an approximation
// of the games' look: it reads the material the way the games read it, and
// lights it with a studio of lights and the light of a sky. What the games'
// own shaders would do beyond that, it tells from the name of the effect a
// part's settings give, which the three games name alike: whether the
// palette colour is blended in, as into skin; whether what the diffuse map's
// alpha leaves out is cut away, as from leaves and hair; whether it is laid
// over what is behind it, as a decal is; and whether it is seen from both
// sides.

// partLook is how the shader draws a part.
type partLook struct {
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

	// tinted overlays the diffuse map with the part's tint, the colour of a
	// tree's leaves, and foliage overlays what is grey of it with a green of
	// leaves, for a tree whose files name no tint.
	tinted, foliage bool

	// patterned colours the part with the portrait accessory its entity's
	// game data names, where its effect is one of the games' pattern
	// effects: the effect is what lays the pattern, so an accessory on a part
	// drawn with another effect would not be drawn by the game either.
	patterned bool
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
	foliageWords  = []string{"tree", "foliage"}

	// patternWords are the effects that colour a portrait accessory, which
	// Victoria 3 and Crusader Kings 3 both name portrait_attachment_pattern:
	// the pattern is laid by the effect, so one named otherwise draws none.
	patternWords = []string{"pattern"}
)

// lookOf is how the shader draws a part. A part without an effect, such as
// one of a bare mesh file, is drawn plain, with the palette colour.
func lookOf(part *model.Part) partLook {
	name := strings.ToLower(part.Shader)
	if name == "" {
		return partLook{palette: true}
	}

	look := partLook{
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
	look.tinted = part.Textures.Tint != nil
	look.foliage = !look.tinted && containsAny(name, foliageWords)
	look.patterned = containsAny(name, patternWords)

	return look
}

// Style is how the renderer draws a part, for an application to show.
type Style struct {
	// Palette blends the palette colour in, as into skin.
	Palette bool

	// Cutout cuts away what the diffuse map's alpha leaves out, as from
	// leaves and hair.
	Cutout bool

	// Blend lays the part over what is behind it by that alpha, as a decal.
	Blend bool

	// TwoSided shows the part from both sides.
	TwoSided bool

	// Atlas reads the textures by the second set of texture coordinates.
	Atlas bool

	// Tinted colours the part with its tint, as a tree's leaves, and
	// Foliage colours what is grey of it green, as leaves of no tint.
	Tinted, Foliage bool

	// Patterned colours the part with the portrait accessory its entity's
	// game data names, where its effect is one of the games' pattern effects.
	Patterned bool
}

// StyleOf is how the renderer draws a part.
func StyleOf(part *model.Part) Style {
	look := lookOf(part)

	return Style{
		Palette:   look.palette,
		Cutout:    look.cutout,
		Blend:     look.blend,
		TwoSided:  look.twoSided,
		Atlas:     look.atlas,
		Tinted:    look.tinted,
		Foliage:   look.foliage,
		Patterned: look.patterned,
	}
}

func containsAny(name string, words []string) bool {
	for _, word := range words {
		if strings.Contains(name, word) {
			return true
		}
	}

	return false
}

// drawPart draws the pieces of a part.
func (m *Model) drawPart(part *gpuPart, transform rl.Matrix) {
	r := m.renderer
	look := part.look

	// What raylib has batched is drawn first, so that it is not drawn the
	// way this part is.
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
	rl.SetShaderValue(r.shader, r.tinted, []float32{boolFloat(look.tinted)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.foliage, []float32{boolFloat(look.foliage)}, rl.ShaderUniformFloat)

	// A part is coloured by its accessory only where its effect lays a
	// pattern, so a part with the data of an accessory its effect knows
	// nothing of is drawn as it was.
	m.drawAccessory(part)

	for _, piece := range part.pieces {
		rl.DrawMesh(*piece.mesh, *part.material, transform)
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

// drawAccessory hands the accessory a part is coloured with to the shader, and
// tells it that there is none where the part has none or its effect lays no
// pattern.
//
// The accessory's textures go to texture units of their own, above the four
// raylib binds a material's maps to, since a material has room for that many
// and no more; the shader is told which unit holds which. It has to run while
// this renderer's shader is the one in use.
func (m *Model) drawAccessory(part *gpuPart) {
	r := m.renderer

	if part.accessory == nil || !part.look.patterned {
		rl.SetShaderValue(r.shader, r.accessory, []float32{0}, rl.ShaderUniformFloat)

		return
	}

	accessory := part.accessory

	rl.EnableShader(r.shader.ID)

	for at, one := range [accessoryUnits]struct {
		location int32
		texture  rl.Texture2D
	}{
		{r.accessoryMask, accessory.mask},
		{r.accessoryPattern[0], accessory.pattern[0]},
		{r.accessoryPattern[1], accessory.pattern[1]},
		{r.accessoryPattern[2], accessory.pattern[2]},
		{r.accessoryPattern[3], accessory.pattern[3]},
		{r.accessoryPalette, accessory.palette},
	} {
		unit := int32(accessoryUnit + at)

		rl.ActiveTextureSlot(unit)
		rl.EnableTexture(one.texture.ID)

		if one.location > -1 {
			rl.SetUniform(one.location, []int32{unit}, int32(rl.ShaderUniformInt), 1)
		}
	}

	rl.SetShaderValue(r.shader, r.accessory, []float32{1}, rl.ShaderUniformFloat)

	layouts := make([]float32, 0, pattern.Channels*4)
	for _, layout := range accessory.layouts {
		layouts = append(layouts, layout[:]...)
	}

	rl.SetShaderValueV(r.shader, r.accessoryLayout, layouts, rl.ShaderUniformVec4, pattern.Channels)

	paletteUv := make([]float32, 0, pattern.Channels*2)
	for _, uv := range accessory.paletteUv {
		paletteUv = append(paletteUv, uv[:]...)
	}

	rl.SetShaderValueV(r.shader, r.accessoryPaletteUv, paletteUv, rl.ShaderUniformVec2, pattern.Channels)
}

// The blend factors of OpenGL that blendOver sets.
const (
	glOne              = 1
	glSrcAlpha         = 0x0302
	glOneMinusSrcAlpha = 0x0303
	glFuncAdd          = 0x8006
)

// blendOver blends what is drawn over what is there by its alpha: the
// colours by it, and the alpha the way one layer covers another, so that
// half of a decal over the solid ground leaves the ground opaque. raylib's
// own alpha blending multiplies the alpha by itself as it does the colours,
// which leaves such a pixel three quarters opaque, the background showing
// through. rl.SetBlendMode(rl.BlendAlpha) goes back to raylib's.
func blendOver() {
	rl.SetBlendFactorsSeparate(glSrcAlpha, glOneMinusSrcAlpha, glOne, glOneMinusSrcAlpha, glFuncAdd, glFuncAdd)
	rl.SetBlendMode(rl.BlendCustomSeparate)
}
