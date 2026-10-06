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

	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/shader"
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

	// The switches of the shader set for every part; see fallback.go.
	usePalette, cutout, blended, noMetal, atlas int32

	white, flat, plain rl.Texture2D

	// The games' own shaders, once UseShaders has been called: the library
	// of their files, the effects compiled from them, and the effects that
	// did not compile, with why.
	shaders      *shader.Library
	source       shader.Source
	effects      map[string]*effect
	effectErrors map[string]error

	// files are the textures samplers name for themselves, by their path,
	// whiteCube what a cube sampler reads, and shadowMap what a shadow
	// sampler reads.
	files     map[string]rl.Texture2D
	whiteCube rl.Texture2D
	blackCube rl.Texture2D
	shadowMap rl.Texture2D

	// shadows is the shadow map of the frame being drawn; see shadow.go.
	shadows shadowMap

	// none is the transparent black what the engine lays over a model
	// stands in as, and mapFiles the textures of the game's map by the name
	// the engine binds them to; see standIn.
	none     rl.Texture2D
	grey     rl.Texture2D
	mapFiles map[string]string

	// frame is the camera of the picture being drawn.
	frame frame

	// postCompiled is the post effect for the environment, or postErr why
	// there is none; see post.
	postCompiled *postEffect
	postErr      error

	// markNonFinite has the effects draw magenta where their colour is not
	// a number, for tests that look for missing defaults.
	markNonFinite bool

	// environment lights the effects, and environmentMap is its cube map.
	environment    *environment.Environment
	environmentMap rl.Texture2D

	// environmentSRGB says the environment map is converted from sRGB as it
	// is sampled; see cubeFor.
	environmentSRGB bool
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
		shader:            shader,
		paletteColor:      rl.GetShaderLocation(shader, "paletteColor"),
		colorMaskInterval: rl.GetShaderLocation(shader, "colorMaskInterval"),
		viewPosition:      rl.GetShaderLocation(shader, "viewPosition"),
		usePalette:        rl.GetShaderLocation(shader, "usePalette"),
		cutout:            rl.GetShaderLocation(shader, "cutout"),
		blended:           rl.GetShaderLocation(shader, "blended"),
		noMetal:           rl.GetShaderLocation(shader, "noMetal"),
		atlas:             rl.GetShaderLocation(shader, "atlas"),
	}

	for _, stand := range []struct {
		texture *rl.Texture2D
		color   [4]byte
	}{
		{&renderer.white, whitePixel},
		{&renderer.flat, flatNormal},
		{&renderer.plain, plainProperties},
		{&renderer.none, [4]byte{}},
		{&renderer.grey, [4]byte{128, 128, 128, 255}},
	} {
		uploaded, err := uploadPixel(stand.color)
		if err != nil {
			renderer.Unload()

			return nil, err
		}

		*stand.texture = uploaded
	}

	// A cube map of six white pixels, one above the other.
	cube := rl.GenImageColor(1, 6, rl.White)
	renderer.whiteCube = rl.LoadTextureCubemap(cube, rl.CubemapLayoutLineVertical)
	rl.UnloadImage(cube)

	black := rl.GenImageColor(1, 6, rl.Black)
	renderer.blackCube = rl.LoadTextureCubemap(black, rl.CubemapLayoutLineVertical)
	rl.UnloadImage(black)

	renderer.shadowMap = litShadowMap()

	return renderer, nil
}

// Unload releases the shader and the stand in textures. Models uploaded with
// the renderer have to be unloaded first.
func (r *Renderer) Unload() {
	r.releaseEffects()
	r.releasePost()
	r.unloadEnvironment()
	r.releaseShadows()

	for _, stand := range []*rl.Texture2D{&r.white, &r.flat, &r.plain, &r.none, &r.grey, &r.whiteCube, &r.blackCube, &r.shadowMap} {
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
