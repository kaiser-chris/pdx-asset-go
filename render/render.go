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
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

//go:embed shaders
var shaders embed.FS

// Renderer holds what every model is drawn with.
type Renderer struct {
	shader rl.Shader

	// The places in the shader of the values set for every model.
	paletteColor      int32
	colorMaskInterval int32
	viewPosition      int32

	// The switches of the shader set for every part; see look.go.
	usePalette, cutout, blended, noMetal, atlas, tinted, foliage int32

	// The uniforms of the accessory a part is coloured with. Its textures are
	// bound to texture units of their own, above the four raylib binds a
	// material's maps to.
	accessory, accessoryLayout, accessoryPaletteUv int32
	accessoryMask, accessoryPalette                int32
	accessoryPattern                               [4]int32

	white, flat, plain rl.Texture2D
}

// accessoryUnit is the texture unit the accessory's mask is bound to, and
// accessoryUnits how many the accessory needs: the mask, the pattern of each
// of the four channels of the mask, and the palette.
const (
	accessoryUnit  = 4
	accessoryUnits = 6
)

// accessoryPatternUniform is the name in the shader of the sampler holding the
// pattern of one channel of the mask.
func accessoryPatternUniform(channel int) string {
	return fmt.Sprintf("accessoryPattern%d", channel)
}

// Look is what a model is drawn with beyond its own textures.
type Look struct {
	// PaletteColor is blended into a part where its diffuse map's colour
	// mask says, such as a skin tone into skin, as red, green and blue from 0
	// to 1.
	PaletteColor [3]float32

	// ColorMaskInterval is the part of the blend the colour mask spans, from
	// the entity's game data. The games default to all of it.
	ColorMaskInterval [2]float32
}

// DefaultLook is a light skin tone, blended in over the whole of the mask.
var DefaultLook = Look{PaletteColor: [3]float32{0.96, 0.8, 0.69}, ColorMaskInterval: [2]float32{0, 1}}

// NewRenderer compiles the shader and uploads the stand in textures. It needs
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
		shader:             shader,
		paletteColor:       rl.GetShaderLocation(shader, "paletteColor"),
		colorMaskInterval:  rl.GetShaderLocation(shader, "colorMaskInterval"),
		viewPosition:       rl.GetShaderLocation(shader, "viewPosition"),
		usePalette:         rl.GetShaderLocation(shader, "usePalette"),
		cutout:             rl.GetShaderLocation(shader, "cutout"),
		blended:            rl.GetShaderLocation(shader, "blended"),
		noMetal:            rl.GetShaderLocation(shader, "noMetal"),
		atlas:              rl.GetShaderLocation(shader, "atlas"),
		tinted:             rl.GetShaderLocation(shader, "tinted"),
		foliage:            rl.GetShaderLocation(shader, "foliage"),
		accessory:          rl.GetShaderLocation(shader, "accessory"),
		accessoryLayout:    rl.GetShaderLocation(shader, "accessoryLayout"),
		accessoryPaletteUv: rl.GetShaderLocation(shader, "accessoryPaletteUv"),
		accessoryMask:      rl.GetShaderLocation(shader, "accessoryMask"),
		accessoryPalette:   rl.GetShaderLocation(shader, "accessoryPalette"),
	}

	for channel := range renderer.accessoryPattern {
		renderer.accessoryPattern[channel] = rl.GetShaderLocation(shader, accessoryPatternUniform(channel))
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

// Unload releases the shader and the stand in textures. Models uploaded with
// the renderer have to be unloaded first.
func (r *Renderer) Unload() {
	for _, stand := range []*rl.Texture2D{&r.white, &r.flat, &r.plain} {
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
	rl.SetShaderValue(r.shader, r.paletteColor, look.PaletteColor[:], rl.ShaderUniformVec3)
	rl.SetShaderValue(r.shader, r.colorMaskInterval, look.ColorMaskInterval[:], rl.ShaderUniformVec2)
	rl.SetShaderValue(r.shader, r.viewPosition, []float32{camera.X, camera.Y, camera.Z}, rl.ShaderUniformVec3)
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
