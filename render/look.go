package render

import (
	"slices"
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

	// coverage draws the part as the games' alpha to coverage does on a
	// picture of several samples to a pixel: solid where the diffuse map's
	// alpha is whole, and laid over what is behind it by that alpha
	// elsewhere, as the lenses of glasses are; see drawStage.
	coverage bool

	// paletteKind is which of the look's palette colours is blended in, and
	// what says where; see the palette constants.
	paletteKind paletteKind

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

	// shadowless leaves the part out of the shadows, as the games leave the
	// parts whose effect has no shadow effect beside it in portrait.shader,
	// such as the eyes and the eyelashes, which would darken the cheeks.
	shadowless bool
}

// Words in the names of the games' effects, and what they say of how a part
// is drawn: portrait_skin, standard_alpha_to_coverage, tree_colormap,
// portrait_hair, decal_local, standard_alpha_blend, standard_two_sided.
var (
	paletteWords  = []string{"skin"}
	eyeWords      = []string{"eye"}
	hairWords     = []string{"hair"}
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

	// shadowlessEffects are the effects of the games' portrait.shader that
	// have no shadow effect, named as theirs with Shadow after: the part of
	// one casts no shadow. They are the same in Victoria 3 and Europa
	// Universalis 5.
	shadowlessEffects = []string{
		"portrait_eye", "portrait_teeth", "portrait_hair_alpha", "portrait_hair_double_sided",
		"portrait_hair_transparency_hack", "portrait_hair_backside", "portrait_attachment_alpha_to_coverage_billboarded",
	}

	// coverageWords are the effects of portrait attachments the games draw
	// with alpha to coverage, portrait_attachment_alpha_to_coverage and its
	// patterned kin, which they keep for glass and lace. Hair is drawn with
	// it as well, but is cut out, which leaves its strands crisp.
	coverageWords = []string{"attachment"}
)

// lookOf is how the shader draws a part. A part without an effect, such as
// one of a bare mesh file, is drawn plain, with the palette colour.
func lookOf(part *model.Part) partLook {
	name := strings.ToLower(part.Shader)
	if name == "" {
		return partLook{palette: true}
	}

	look := partLook{
		palette:  containsAny(name, paletteWords) || containsAny(name, eyeWords) || containsAny(name, hairWords),
		cutout:   containsAny(name, cutoutWords),
		twoSided: containsAny(name, twoSidedWords),
		noMetal:  containsAny(name, noMetalWords),
		atlas:    containsAny(name, atlasWords),
	}

	// A cut away part is not blended as well: the effects of hair and
	// leaves cut by their alpha and write depth, and the alpha in their
	// names is that cut.
	if containsAny(name, []string{"alpha_to_coverage"}) && containsAny(name, coverageWords) {
		look.coverage, look.cutout = true, false
	}

	look.blend = part.IsDecal() || !look.cutout && !look.coverage && containsAny(name, blendWords)
	look.tinted = part.Textures.Tint != nil
	look.foliage = !look.tinted && containsAny(name, foliageWords)
	look.patterned = containsAny(name, patternWords)
	look.paletteKind = paletteKindOf(name)
	look.shadowless = slices.Contains(shadowlessEffects, name)

	return look
}

// Style is how the renderer draws a part, for an application to show.
type Style struct {
	// Palette blends a palette colour in: the skin colour into skin, the
	// eye colour into eyes and the hair colour into hair.
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

// drawStage is which of the two stages a part drawn with alpha to coverage
// is drawn in: its solid pixels first, with every other solid part, and then
// the rest of them laid over all of that, without hiding what lies behind
// them from what is laid after. The graphics card does this in one go on a
// picture of several samples to a pixel, which the viewer's picture is not.
type drawStage int

const (
	stageSolid drawStage = iota
	stageTranslucent
)

// drawPart draws the pieces of a part in a stage.
func (m *Model) drawPart(part *gpuPart, transform rl.Matrix, stage drawStage) {
	r := m.renderer
	look := part.look

	// The translucent stage of a part drawn with alpha to coverage is laid
	// over what is there as a blended part is.
	if look.coverage && stage == stageTranslucent {
		look.blend = true
	}

	coverage := float32(0)
	if look.coverage {
		coverage = float32(stage) + 1
	}

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

	// A diffuse map decals were laid over after the skin colour has the
	// colour blended in already.
	palette := look.palette && (part.decals == nil || !part.decals.paletted)

	rl.SetShaderValue(r.shader, r.usePalette, []float32{boolFloat(palette)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.paletteKind, []float32{float32(look.paletteKind)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.cutout, []float32{boolFloat(look.cutout)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.coverage, []float32{coverage}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.blended, []float32{boolFloat(look.blend)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.noMetal, []float32{boolFloat(look.noMetal)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.atlas, []float32{boolFloat(look.atlas)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.tinted, []float32{boolFloat(look.tinted)}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.foliage, []float32{boolFloat(look.foliage)}, rl.ShaderUniformFloat)

	interval := colorMaskInterval(part, r.lookInterval)
	rl.SetShaderValue(r.shader, r.colorMaskInterval, interval[:], rl.ShaderUniformVec2)

	// A part is coloured by its accessory only where its effect lays a
	// pattern, so a part with the data of an accessory its effect knows
	// nothing of is drawn as it was.
	m.drawAccessory(part)
	m.bindOcclusionColour(part)

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

// drawAccessory hands the accessory a part is coloured and surfaced with to the
// shader, and tells it that there is none where the part has none or its
// effect lays no pattern.
//
// The mask goes in the texture unit raylib keeps for a material's roughness
// map, which a part drawn with a pattern puts no tint in: raylib binds it
// there as it draws the part, so the shader is only told which unit that is.
// The pattern of each channel, with the surface it brings, goes to the units
// above the four raylib binds a material's own maps to, since a material has
// room for that many and no more. It has to run while this renderer's shader
// is the one in use.
func (m *Model) drawAccessory(part *gpuPart) {
	r := m.renderer

	if part.accessory == nil || !part.look.patterned {
		rl.SetShaderValue(r.shader, r.accessory, []float32{0}, rl.ShaderUniformFloat)

		return
	}

	accessory := part.accessory

	rl.EnableShader(r.shader.ID)

	rl.SetMaterialTexture(part.material, rl.MapRoughness, accessory.mask)

	if r.accessoryMask > -1 {
		rl.SetUniform(r.accessoryMask, []int32{accessoryMaskUnit}, int32(rl.ShaderUniformInt), 1)
	}

	type bound struct {
		location int32
		texture  rl.Texture2D
	}

	var textures [accessoryUnits]bound

	for channel := range pattern.Channels {
		textures[channel] = bound{r.accessoryPattern[channel], accessory.pattern[channel]}
		textures[pattern.Channels+channel] = bound{r.accessoryNormal[channel], accessory.normal[channel]}
		textures[2*pattern.Channels+channel] = bound{r.accessoryProperties[channel], accessory.properties[channel]}
	}

	for at, one := range textures {
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

	colours := make([]float32, 0, pattern.Channels*3)
	for _, colour := range accessory.colours {
		colours = append(colours, colour[:]...)
	}

	rl.SetShaderValueV(r.shader, r.accessoryColours, colours, rl.ShaderUniformVec3, pattern.Channels)
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

// paletteKind is which palette colour of a look a part is coloured with, and
// what says where, as the games' portrait shaders do.
type paletteKind int

const (
	// paletteSkin blends the skin colour in where the diffuse map's alpha
	// says, as portrait_skin does.
	paletteSkin paletteKind = iota

	// paletteEye blends the eye colour in where the diffuse map's alpha says,
	// as portrait_eye does.
	paletteEye

	// paletteHair blends the hair colour in where the blue of the normal map
	// says, as portrait_hair does: the alpha of a hair's diffuse map is what
	// cuts its strands out, so the mask is kept in the normal map, whose
	// blue a normal reconstructed from red and green has no use for.
	paletteHair

	// paletteHairWhole multiplies the whole of the diffuse map by the hair
	// colour, as portrait_hair_double_sided does.
	paletteHairWhole
)

// paletteKindOf is the palette colour the effect of a name colours a part
// with: an eye's, a hair's, or by default a skin's.
func paletteKindOf(effect string) paletteKind {
	switch {
	case containsAny(effect, hairWords) && containsAny(effect, []string{"double_sided"}):
		return paletteHairWhole
	case containsAny(effect, hairWords):
		return paletteHair
	case containsAny(effect, eyeWords):
		return paletteEye
	default:
		return paletteSkin
	}
}

// colorMaskInterval is the interval a part's colour mask is remapped into:
// its entity's, or the look's where the entity gives none. The games give
// each entity its own, such as 0.9 to 1 for hair that is coloured almost
// wholly however little its mask says.
func colorMaskInterval(part *gpuPart, look [2]float32) [2]float32 {
	if part.colorMaskInterval != nil {
		return *part.colorMaskInterval
	}

	return look
}
