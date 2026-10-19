// Package render draws the models of the model package with raylib.
//
// A Renderer holds what every model is drawn with: the shader and the neutral
// textures a part without one of its own falls back to. Upload hands a model
// to the GPU, and a Viewer draws models into a picture of their own from a
// camera that circles them, which is what an interface shows and turns.
//
// Everything here needs the OpenGL context raylib created with its window,
// and has to run on the thread that owns it.
package render

import (
	"embed"
	"fmt"
	"math"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed shaders
var shaders embed.FS

// Renderer holds what every model is drawn with.
type Renderer struct {
	shader rl.Shader

	// lit are the places in the shader of the uniforms of a Lighting; see
	// environment.go.
	lit lightingLocations

	// The places in the shader of the values set for every model.
	paletteColor      int32
	eyeColor          int32
	hairColor         int32
	highlight         int32
	colorMaskInterval int32
	viewPosition      int32

	// The switches of the shader set for every part; see look.go.
	usePalette, paletteKind, cutout, coverage, blended, noMetal, atlas, tinted, foliage int32

	// The uniforms of the accessory a part is coloured and surfaced with. Its
	// textures are bound to texture units of their own: the mask to the unit
	// raylib keeps for a material's roughness map, which nothing else here
	// puts a texture in, and the rest above the four units raylib binds a
	// material's maps to.
	accessory, accessoryLayout, accessoryColours int32
	accessoryMask                                int32
	accessoryPattern                             [4]int32
	accessoryNormal                              [4]int32
	accessoryProperties                          [4]int32

	white, flat, plain, black rl.Texture2D

	// decals is the shader decals are laid over the skin with, and scratch
	// the targets they are laid between, by the size of the maps; see
	// decal.go.
	decals  decalShader
	scratch map[[2]int32]*scratch

	// occlusion is the shader the ambient occlusion is worked out with, and
	// the rest the places in the model shader of what lays it over the
	// light; see occlusion.go.
	occlusion                                                       occlusionShader
	occluded, occlusionBlend, occlusionResolution                   int32
	occlusionMap, occlusionColour, occlusionTinted, occlusionMapped int32
}

// The texture units the accessory's textures are bound to: the unit of a
// material's roughness map, which a part drawn with a pattern puts no tint in,
// and then the units above the four raylib binds a material's own maps to.
//
// Sixteen texture units are what an OpenGL 3.3 context is sure to have, which
// is the one raylib asks for: one for each of the three textures of a channel
// of the mask, one for the mask, and the four raylib itself uses.
const (
	accessoryUnit  = 4
	accessoryUnits = 12

	// accessoryMaskUnit is where the mask goes: the texture unit raylib keeps
	// for a material's roughness map, which a part drawn with a pattern puts
	// no tint in.
	accessoryMaskUnit = 3
)

// The names in the shader of the samplers holding the pattern, the normal map
// and the properties map of one channel of the mask.
func accessoryPatternUniform(channel int) string {
	return fmt.Sprintf("accessoryPattern%d", channel)
}

func accessoryNormalUniform(channel int) string {
	return fmt.Sprintf("accessoryNormal%d", channel)
}

func accessoryPropertiesUniform(channel int) string {
	return fmt.Sprintf("accessoryProperties%d", channel)
}

// Look is what a model is drawn with beyond its own textures.
type Look struct {
	// PaletteColor is blended into a part where its diffuse map's colour
	// mask says, such as a skin tone into skin, as red, green and blue from 0
	// to 1.
	PaletteColor [3]float32

	// Lighting lights the models in the place of the renderer's own
	// studio of lights, nil for those.
	Lighting *Lighting

	// ColorMaskInterval is the part of the blend the colour mask spans, from
	// the entity's game data. The games default to all of it.
	ColorMaskInterval [2]float32

	// EyeColor and HairColor are blended into eyes and hair the way
	// PaletteColor is into skin: the games give a portrait three palette
	// colours, one for each, and pick the one by the effect a part is drawn
	// with, portrait_eye, portrait_hair or portrait_skin.
	EyeColor, HairColor [3]float32

	// Highlight is laid over the vertices Model.Highlight marks, as red,
	// green and blue from 0 to 1 and how strongly, at most, from 0 to 1.
	Highlight [4]float32
}

// DefaultLook is a light skin tone, brown eyes and hair, blended in over the
// whole of the mask, and highlights in a light blue, which no skin tone is
// near.
var DefaultLook = Look{
	PaletteColor:      [3]float32{0.96, 0.8, 0.69},
	EyeColor:          [3]float32{0.6, 0.48, 0.36},
	HairColor:         [3]float32{0.45, 0.36, 0.3},
	ColorMaskInterval: [2]float32{0, 1},
	Highlight:         [4]float32{0.4, 0.75, 1, 0.65},
}

// NewRenderer compiles the shaders and uploads the stand in textures. It needs
// the OpenGL context.
func NewRenderer() (*Renderer, error) {
	vertex, err := shaders.ReadFile("shaders/standard.vs")
	if err != nil {
		return nil, err
	}

	fragment, err := shaders.ReadFile("shaders/standard.fs")
	if err != nil {
		return nil, err
	}

	shader := rl.LoadShaderFromMemory(string(vertex), string(fragment))
	if shader.ID == 0 || shader.ID == rl.GetShaderIdDefault() {
		return nil, fmt.Errorf("the model shader did not compile")
	}

	renderer := &Renderer{
		shader:              shader,
		paletteColor:        rl.GetShaderLocation(shader, "paletteColor"),
		eyeColor:            rl.GetShaderLocation(shader, "eyeColor"),
		hairColor:           rl.GetShaderLocation(shader, "hairColor"),
		highlight:           rl.GetShaderLocation(shader, "highlightColor"),
		paletteKind:         rl.GetShaderLocation(shader, "paletteKind"),
		colorMaskInterval:   rl.GetShaderLocation(shader, "colorMaskInterval"),
		viewPosition:        rl.GetShaderLocation(shader, "viewPosition"),
		usePalette:          rl.GetShaderLocation(shader, "usePalette"),
		cutout:              rl.GetShaderLocation(shader, "cutout"),
		coverage:            rl.GetShaderLocation(shader, "coverage"),
		blended:             rl.GetShaderLocation(shader, "blended"),
		noMetal:             rl.GetShaderLocation(shader, "noMetal"),
		atlas:               rl.GetShaderLocation(shader, "atlas"),
		tinted:              rl.GetShaderLocation(shader, "tinted"),
		foliage:             rl.GetShaderLocation(shader, "foliage"),
		accessory:           rl.GetShaderLocation(shader, "accessory"),
		accessoryLayout:     rl.GetShaderLocation(shader, "accessoryLayout"),
		accessoryColours:    rl.GetShaderLocation(shader, "accessoryColours"),
		accessoryMask:       rl.GetShaderLocation(shader, "accessoryMask"),
		occluded:            rl.GetShaderLocation(shader, "occluded"),
		occlusionBlend:      rl.GetShaderLocation(shader, "occlusionBlend"),
		occlusionResolution: rl.GetShaderLocation(shader, "occlusionResolution"),
		occlusionMap:        rl.GetShaderLocation(shader, "occlusionMap"),
		occlusionColour:     rl.GetShaderLocation(shader, "occlusionColour"),
		occlusionTinted:     rl.GetShaderLocation(shader, "occlusionTinted"),
		occlusionMapped:     rl.GetShaderLocation(shader, "occlusionMapped"),
	}

	renderer.lit = locateLighting(shader)

	decals, err := loadDecalShader()
	if err != nil {
		rl.UnloadShader(shader)

		return nil, err
	}

	renderer.decals = decals

	occlusion, err := loadOcclusionShader()
	if err != nil {
		renderer.Unload()

		return nil, err
	}

	renderer.occlusion = occlusion

	for channel := range renderer.accessoryPattern {
		renderer.accessoryPattern[channel] = rl.GetShaderLocation(shader, accessoryPatternUniform(channel))
		renderer.accessoryNormal[channel] = rl.GetShaderLocation(shader, accessoryNormalUniform(channel))
		renderer.accessoryProperties[channel] = rl.GetShaderLocation(shader, accessoryPropertiesUniform(channel))
	}

	// raylib binds the diffuse, specular and normal maps of a material to
	// texture0 to texture2 by itself. The tint goes in the slot of the
	// roughness map, which raylib binds once it knows where it goes.
	shader.UpdateLocation(rl.ShaderLocMapRoughness, rl.GetShaderLocation(shader, "texture3"))

	for _, stand := range []struct {
		texture *rl.Texture2D
		color   [4]byte
	}{
		{&renderer.white, whitePixel},
		{&renderer.flat, flatNormal},
		{&renderer.plain, plainProperties},
		{&renderer.black, blackPixel},
	} {
		uploaded, err := uploadPixel(stand.color)
		if err != nil {
			renderer.Unload()

			return nil, err
		}

		*stand.texture = uploaded
	}

	return renderer, nil
}

// Unload releases the shaders, the stand in textures and the targets decals
// were laid between. Models uploaded with the renderer have to be unloaded
// first.
func (r *Renderer) Unload() {
	r.unloadDecals()
	r.occlusion.unload()

	for _, stand := range []*rl.Texture2D{&r.white, &r.flat, &r.plain, &r.black} {
		if stand.ID != 0 {
			rl.UnloadTexture(*stand)
			*stand = rl.Texture2D{}
		}
	}

	if r.shader.ID != 0 {
		rl.UnloadShader(r.shader)
		r.shader = rl.Shader{}
	}
}

// apply sets the values of a look and a camera position on the shader.
func (r *Renderer) apply(look Look, camera rl.Vector3) {
	// The values are copied out of the look before they are handed to C: a
	// slice of the look itself points into memory that also holds the
	// pointer to its Lighting, which the rules of cgo forbid handing over.
	palette, eye, hair, highlight, interval := look.PaletteColor, look.EyeColor, look.HairColor, look.Highlight, look.ColorMaskInterval

	rl.SetShaderValue(r.shader, r.paletteColor, palette[:], rl.ShaderUniformVec3)
	rl.SetShaderValue(r.shader, r.eyeColor, eye[:], rl.ShaderUniformVec3)
	rl.SetShaderValue(r.shader, r.hairColor, hair[:], rl.ShaderUniformVec3)
	rl.SetShaderValue(r.shader, r.highlight, highlight[:], rl.ShaderUniformVec4)
	rl.SetShaderValue(r.shader, r.colorMaskInterval, interval[:], rl.ShaderUniformVec2)
	rl.SetShaderValue(r.shader, r.viewPosition, []float32{camera.X, camera.Y, camera.Z}, rl.ShaderUniformVec3)
	r.applyLighting(look.Lighting)
}

// releaseMaterial frees a material without the shader and textures it draws
// with, which belong to the renderer and the model.
//
// raylib's UnloadMaterial unloads every texture of a material and its shader
// along with the material itself. Shared as they are here, that would unload
// them under every other material that uses them, and unload them a second
// time when their owner does, which deletes whatever texture the GPU has given
// their number to since. So the material is emptied first: number zero is
// nothing, which OpenGL ignores.
func releaseMaterial(material *rl.Material) {
	material.Shader = rl.Shader{}

	if material.Maps != nil {
		maps := unsafe.Slice(material.Maps, materialMaps)
		for index := range maps {
			maps[index].Texture = rl.Texture2D{}
		}
	}

	rl.UnloadMaterial(*material)
}

// materialMaps is how many maps raylib gives every material, MAX_MATERIAL_MAPS
// in raylib.h.
const materialMaps = 12

// PaletteColour turns a colour of a palette, such as the skin tone a DNA picks
// from the skin palette, into the PaletteColor of a Look.
//
// The games multiply a diffuse map by the palette colour in linear light,
// with the palette colour as it is stored, and this renderer works on colours
// as they are stored, in sRGB: multiplying by the palette colour taken to the
// power of one over the gamma does in sRGB what multiplying by it does in
// linear light.
func PaletteColour(colour [4]byte) [3]float32 {
	var linear [3]float32

	for channel := range linear {
		linear[channel] = float32(math.Pow(float64(colour[channel])/255, 1/gamma))
	}

	return linear
}

// SRGBPaletteColour turns a colour of a palette the game reads as sRGB into
// the PaletteColor of a Look, as Europa Universalis 5's srgb_color_palettes
// = yes asks: the engine takes the colour into linear light before it
// multiplies a diffuse map by it, which in the sRGB this renderer works in is
// multiplying by the colour as it is stored.
func SRGBPaletteColour(colour [4]byte) [3]float32 {
	return [3]float32{float32(colour[0]) / 255, float32(colour[1]) / 255, float32(colour[2]) / 255}
}

// gamma is the exponent sRGB is taken to approximate.
const gamma = 2.2
