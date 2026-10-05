package render

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/shader"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// The games' own shaders.
//
// A part is drawn with the effect its mesh settings name, built from the
// game's shader files by the shader package and compiled for OpenGL. What
// the asset gives the effect, its textures and its defines, it gets; what the
// engine gives it while the game runs, it gets in the most basic form there
// is, since there is no game running:
//
//   - a texture the asset does not give and the sampler names no file for
//     reads white, or nothing for a kind of texture raylib cannot make, such
//     as an array of textures;
//   - a constant that is a colour, three or four numbers, is white, a matrix
//     is the identity, and every other number is zero;
//   - the camera's constants are the viewer's camera, and the model sits at
//     the origin, fully opaque, the way the viewer places it.

// defaultShaderFile is the shader file of mesh settings that name an effect
// but no file. The engine's choice then is not established; every effect the
// shipped settings name without a file is one of pdxmesh.shader.
const defaultShaderFile = "gfx/FX/pdxmesh.shader"

// UseShaders has models drawn with the games' own shaders, read from a
// source: their shader files and those of their engine. Without it every
// part is drawn with the renderer's own shader.
//
// The effects compiled from an earlier source are released, so the models
// uploaded with them have to be unloaded first.
func (r *Renderer) UseShaders(source shader.Source) {
	r.releaseEffects()

	r.shaders = shader.NewLibrary(source)
	r.source = source
	r.effects = map[string]*effect{}
}

// fileTexture uploads a texture a sampler names for itself, such as the tint
// map of the trees, once. One that cannot be read is white.
func (r *Renderer) fileTexture(name string) rl.Texture2D {
	if found, ok := r.files[name]; ok {
		return found
	}

	if r.files == nil {
		r.files = map[string]rl.Texture2D{}
	}

	uploaded := r.white

	if data, err := r.source.ReadFile(name); err == nil {
		if decoded, err := texture.Decode(name, data); err == nil {
			if done, err := UploadTexture(decoded); err == nil {
				uploaded = done
			}
		}
	}

	r.files[name] = uploaded

	return uploaded
}

// effect is an effect compiled for OpenGL, and where its values go.
type effect struct {
	shader  rl.Shader
	program *shader.Program

	textures []effectTexture
	camera   map[string]int32
	instance [5]int32

	twoSided bool
	blend    bool
	noDepth  bool
}

// effectTexture is a sampler of an effect and the unit it reads from.
type effectTexture struct {
	location int32
	unit     int32
	texture  shader.Texture
}

// effectFor compiles the effect a part is drawn with, once for every set of
// defines.
func (r *Renderer) effectFor(part *model.Part) (*effect, error) {
	file := cmp.Or(part.ShaderFile, defaultShaderFile)

	defines := slices.Clone(part.Defines)
	if part.HasUV1 {
		// The engine's option for a mesh with a second set of texture
		// coordinates.
		defines = append(defines, "PDX_MESH_UV1")
	}

	key := file + "\x00" + part.Shader + "\x00" + strings.Join(defines, "\x00")

	if compiled, ok := r.effects[key]; ok {
		if compiled == nil {
			return nil, r.effectErrors[key]
		}

		return compiled, nil
	}

	compiled, err := r.compileEffect(file, part.Shader, defines)
	if err != nil {
		if r.effectErrors == nil {
			r.effectErrors = map[string]error{}
		}

		r.effects[key] = nil
		r.effectErrors[key] = err

		return nil, err
	}

	r.effects[key] = compiled

	return compiled, nil
}

func (r *Renderer) compileEffect(file, name string, defines []string) (*effect, error) {
	program, err := r.shaders.Program(file, name, defines)
	if err != nil {
		return nil, err
	}

	compiled := rl.LoadShaderFromMemory(program.Vertex, program.Pixel)
	if compiled.ID == 0 || compiled.ID == rl.GetShaderIdDefault() {
		return nil, fmt.Errorf("effect %s of %s: the driver did not take the program", name, file)
	}

	built := &effect{shader: compiled, program: program, camera: map[string]int32{}, instance: [5]int32{-1, -1, -1, -1, -1}}

	for unit, texture := range program.Textures {
		location := rl.GetShaderLocation(compiled, texture.Name)
		if location < 0 {
			continue
		}

		// Every sampler reads from a unit of its own: OpenGL refuses to draw
		// with two kinds of sampler on one unit, which is what samplers
		// left at their default unit would be.
		built.textures = append(built.textures, effectTexture{location: location, unit: int32(unit), texture: texture})
		rl.SetShaderValue(compiled, location, []float32{math.Float32frombits(uint32(unit))}, rl.ShaderUniformInt)
	}

	for _, uniform := range program.Uniforms {
		location := rl.GetShaderLocation(compiled, uniform.Name)
		if location < 0 {
			continue
		}

		buffer, member, _ := strings.Cut(uniform.Name, ".")

		switch {
		case buffer == "cb_PdxCamera":
			built.camera[member] = location
		case buffer == "cb_PdxMeshInstanceData" && strings.HasPrefix(member, "Data["):
			var index int
			if _, err := fmt.Sscanf(member, "Data[%d]", &index); err == nil && index < len(built.instance) {
				built.instance[index] = location
			}
		}

		setDefault(compiled, location, uniform)
	}

	if state := program.States["RasterizerState"]; state != nil && strings.EqualFold(state.Values["CullMode"], "none") {
		built.twoSided = true
	}

	if state := program.States["BlendState"]; state != nil && strings.EqualFold(state.Values["BlendEnable"], "yes") {
		built.blend = true
	}

	if state := program.States["DepthStencilState"]; state != nil && strings.EqualFold(state.Values["DepthWriteEnable"], "no") {
		built.noDepth = true
	}

	return built, nil
}

// setDefault gives a constant the engine sets its most basic value: white
// for a colour, the identity for a matrix, and zero, which OpenGL starts a
// uniform at, for everything else.
func setDefault(target rl.Shader, location int32, uniform shader.Uniform) {
	if uniform.Kind != shader.KindFloat {
		return
	}

	switch {
	case uniform.Columns == 4 && uniform.VectorSize == 4:
		rl.SetShaderValueMatrix(target, location, rl.MatrixIdentity())
	case uniform.Columns == 1 && uniform.VectorSize == 3:
		rl.SetShaderValue(target, location, []float32{1, 1, 1}, rl.ShaderUniformVec3)
	case uniform.Columns == 1 && uniform.VectorSize == 4:
		rl.SetShaderValue(target, location, []float32{1, 1, 1, 1}, rl.ShaderUniformVec4)
	}
}

// frame is what the effects need to know of the camera a frame is drawn
// with.
type frame struct {
	view, projection rl.Matrix
	camera           rl.Camera3D
	near, far        float32
}

// drawEffect draws the pieces of a part with its effect.
func (m *Model) drawEffect(part *gpuPart, transform rl.Matrix) {
	compiled := part.effect
	current := m.renderer.frame

	// What raylib has batched is drawn first, so that it is not drawn with
	// this effect.
	rl.DrawRenderBatchActive()
	rl.EnableShader(compiled.shader.ID)

	viewProjection := rl.MatrixMultiply(current.view, current.projection)

	// The games' shaders multiply a vector by a matrix from the left, the
	// way HLSL keeps its matrices, which raylib keeps the other way round.
	matrices := map[string]rl.Matrix{
		"ViewProjectionMatrix":    viewProjection,
		"InvViewProjectionMatrix": rl.MatrixInvert(viewProjection),
		"ViewMatrix":              current.view,
		"InvViewMatrix":           rl.MatrixInvert(current.view),
		"ProjectionMatrix":        current.projection,
		"InvProjectionMatrix":     rl.MatrixInvert(current.projection),
	}

	for name, value := range matrices {
		if location, ok := compiled.camera[name]; ok {
			rl.SetShaderValueMatrix(compiled.shader, location, rl.MatrixTranspose(value))
		}
	}

	camera := current.camera
	forward := rl.Vector3Normalize(rl.Vector3Subtract(camera.Target, camera.Position))
	right := rl.Vector3Normalize(rl.Vector3CrossProduct(forward, camera.Up))
	up := rl.Vector3CrossProduct(right, forward)

	for name, value := range map[string][]float32{
		"CameraPosition":  {camera.Position.X, camera.Position.Y, camera.Position.Z},
		"CameraLookAtDir": {forward.X, forward.Y, forward.Z},
		"CameraUpDir":     {up.X, up.Y, up.Z},
		"CameraRightDir":  {right.X, right.Y, right.Z},
		"ZNear":           {current.near},
		"ZFar":            {current.far},
		"CameraFoV":       {camera.Fovy * math.Pi / 180},
	} {
		if location, ok := compiled.camera[name]; ok {
			kind := rl.ShaderUniformFloat
			if len(value) == 3 {
				kind = rl.ShaderUniformVec3
			}

			rl.SetShaderValue(compiled.shader, location, value, kind)
		}
	}

	// The instance: its world matrix in the first four elements, a column
	// each, and in the fifth its opacity, fully opaque.
	world := rl.MatrixToFloat(transform)
	for column := range 4 {
		if location := compiled.instance[column]; location >= 0 {
			rl.SetShaderValue(compiled.shader, location, world[column*4:column*4+4], rl.ShaderUniformVec4)
		}
	}

	if location := compiled.instance[4]; location >= 0 {
		rl.SetShaderValue(compiled.shader, location, []float32{1, 0, 0, 0}, rl.ShaderUniformVec4)
	}

	for _, bound := range compiled.textures {
		rl.ActiveTextureSlot(bound.unit)

		switch {
		case strings.HasPrefix(bound.texture.Type, "samplerCube"):
			rl.EnableTextureCubemap(m.renderer.whiteCube.ID)
		case bound.texture.Type == "sampler2D":
			rl.EnableTexture(part.textureFor(bound.texture).ID)
		}
	}

	if compiled.twoSided {
		rl.DisableBackfaceCulling()
	}

	if compiled.blend {
		rl.SetBlendMode(rl.BlendAlpha)
	}

	if compiled.noDepth {
		rl.DisableDepthMask()
	}

	for _, piece := range part.pieces {
		if rl.EnableVertexArray(piece.VaoID) {
			rl.DrawVertexArrayElements(0, piece.TriangleCount*3, nil)
			rl.DisableVertexArray()
		}
	}

	if compiled.noDepth {
		rl.EnableDepthMask()
	}

	if compiled.blend {
		rl.SetBlendMode(rl.BlendAlpha)
	}

	if compiled.twoSided {
		rl.EnableBackfaceCulling()
	}

	for _, bound := range compiled.textures {
		rl.ActiveTextureSlot(bound.unit)
		rl.DisableTexture()
		rl.DisableTextureCubemap()
	}

	rl.ActiveTextureSlot(0)
	rl.DisableShader()
}

// textureFor is the texture a sampler of the part's effect reads: the
// asset's texture of the sampler's slot, the file the sampler names when the
// asset gives none, or white.
func (p *gpuPart) textureFor(texture shader.Texture) rl.Texture2D {
	if texture.Sampler != nil {
		if slot, ok := samplerSlot(texture.Sampler); ok {
			if found, ok := p.slots[slot]; ok {
				return found
			}
		}

		if found, ok := p.files[texture.Sampler.File]; ok && texture.Sampler.File != "" {
			return found
		}
	}

	return p.white
}

// samplerSlot is the slot of the asset's textures a sampler takes: the
// number of its PdxTexture reference, or its index.
func samplerSlot(sampler *shader.Sampler) (int, bool) {
	var slot int
	if _, err := fmt.Sscanf(sampler.Ref, "PdxTexture%d", &slot); err == nil {
		return slot, true
	}

	if sampler.HasIndex && sampler.Ref == "" {
		return sampler.Index, true
	}

	return 0, false
}

// releaseEffects unloads the effects compiled so far and the textures their
// samplers named.
func (r *Renderer) releaseEffects() {
	for _, compiled := range r.effects {
		if compiled != nil {
			rl.UnloadShader(compiled.shader)
		}
	}

	for _, file := range r.files {
		// A file that could not be read stands in as the white pixel, which
		// belongs to the renderer.
		if file.ID != r.white.ID {
			rl.UnloadTexture(file)
		}
	}

	r.effects = map[string]*effect{}
	r.effectErrors = nil
	r.files = nil
}

// UsesOwnShader reports whether any part of the model is drawn with the
// renderer's own shader rather than an effect of the game's, which is what
// the Look of a viewer applies to.
func (m *Model) UsesOwnShader() bool {
	for _, part := range m.parts {
		if part.effect == nil {
			return true
		}
	}

	return false
}
