package render

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
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
	r.releasePost()

	r.shaders = shader.NewLibrary(source)
	r.source = source
	r.mapFiles = nil
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

	textures    []effectTexture
	camera      map[string]int32
	environment map[string]int32

	// shared are the constants of the effect the graphics defines may set,
	// by the member's name.
	shared   map[string]sharedConstant
	instance [5]int32

	// shadow are the constants of PdxShadowmap, by their member's name.
	shadow map[string]int32

	twoSided bool
	blend    bool
	noDepth  bool

	// slope and steps offset the depth the effect draws; see
	// rasterizerBias.
	slope, steps float32
}

// sharedConstant is a constant of an effect and where it goes.
type sharedConstant struct {
	location int32
	uniform  shader.Uniform
}

// effectTexture is a sampler of an effect and the unit it reads from.
type effectTexture struct {
	location int32
	unit     int32
	texture  shader.Texture

	// srgb is where the switch is that has what the sampler reads converted
	// from sRGB, or -1; see srgb.go.
	srgb int32
}

// effectFor compiles the effect a part is drawn with, once for every set of
// defines.
func (r *Renderer) effectFor(part *model.Part) (*effect, error) {
	return r.effectNamed(part, part.Shader, part.IsDecal())
}

// shadowEffectFor compiles the effect a part casts its shadow with, or has
// none for a part that casts none, as one whose effect has no effect for
// its shadow in its file, such as a decal's.
func (r *Renderer) shadowEffectFor(part *model.Part) *effect {
	if part.ShadowShader == "" {
		return nil
	}

	compiled, err := r.effectNamed(part, part.ShadowShader, false)
	if err != nil {
		return nil
	}

	return compiled
}

// effectNamed compiles an effect of a part's shader file, with the defines
// the part is drawn with, once for every set of them.
func (r *Renderer) effectNamed(part *model.Part, name string, decal bool) (*effect, error) {
	file := cmp.Or(part.ShaderFile, defaultShaderFile)

	defines := slices.Clone(part.Defines)
	if part.HasUV1 {
		// The engine's option for a mesh with a second set of texture
		// coordinates.
		defines = append(defines, "PDX_MESH_UV1")
	}

	// The engine's option for a model drawn in the interface rather than on
	// the map, which the viewer draws it as: Victoria 3's shaders then leave
	// out the map's overlays, fog and wind, which read what only the map
	// has. The other games do not read it.
	defines = append(defines, "GUI_SHADER")

	key := file + "\x00" + name + "\x00" + strings.Join(defines, "\x00") + "\x00" + fmt.Sprint(decal)

	if compiled, ok := r.effects[key]; ok {
		if compiled == nil {
			return nil, r.effectErrors[key]
		}

		return compiled, nil
	}

	compiled, err := r.compileEffect(file, name, defines, decal)
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

// A decal is drawn over what is solid, blended by its alpha and leaving the
// depth as it is, whatever its effect's states say: the games draw the
// passes of decals so.
func (r *Renderer) compileEffect(file, name string, defines []string, decal bool) (*effect, error) {
	program, err := r.shaders.Program(file, name, defines)
	if err != nil {
		return nil, err
	}

	blend := decal
	if state := program.States["BlendState"]; state != nil && strings.EqualFold(state.Values["BlendEnable"], "yes") {
		blend = true
	}

	var colors []string
	for _, texture := range program.Textures {
		if texture.Type == "sampler2D" || texture.Type == "samplerCube" {
			colors = append(colors, texture.Name)
		}
	}

	pixel := decodeSRGB(program.Pixel, colors)
	if state := program.States["BlendState"]; !blend && state != nil && stateIsSet(state, "AlphaToCoverage") {
		pixel = coverageOutput(pixel)
	}

	if !blend {
		pixel = opaqueOutput(pixel)
	}

	if r.markNonFinite {
		pixel = markNonFinite(pixel)
	}

	compiled := rl.LoadShaderFromMemory(program.Vertex, pixel)
	if compiled.ID == 0 || compiled.ID == rl.GetShaderIdDefault() {
		return nil, fmt.Errorf("effect %s of %s: the driver did not take the program", name, file)
	}

	built := &effect{
		shader:      compiled,
		program:     program,
		camera:      map[string]int32{},
		environment: map[string]int32{},
		shared:      map[string]sharedConstant{},
		shadow:      map[string]int32{},
		instance:    [5]int32{-1, -1, -1, -1, -1},
	}

	for unit, texture := range program.Textures {
		location := rl.GetShaderLocation(compiled, texture.Name)
		if location < 0 {
			continue
		}

		// Every sampler reads from a unit of its own: OpenGL refuses to draw
		// with two kinds of sampler on one unit, which is what samplers
		// left at their default unit would be.
		built.textures = append(built.textures, effectTexture{
			location: location,
			unit:     int32(unit),
			texture:  texture,
			srgb:     rl.GetShaderLocation(compiled, srgbSwitch(texture.Name)),
		})
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
		case buffer == "cb_JominiEnvironment":
			built.environment[member] = location
		case buffer == "cb_PdxMeshInstanceData" && strings.HasPrefix(member, "Data["):
			var index int
			if _, err := fmt.Sscanf(member, "Data[%d]", &index); err == nil && index < len(built.instance) {
				built.instance[index] = location
			}

			// Not colours: what follows the world matrix and the opacity is
			// the instance's user data, such as the standard of living of a
			// building, which a model that has none of reads as zero.
			continue
		case buffer == "cb_PdxShadowmap":
			// Set as the effect is drawn; see applyShadows.
			built.shadow[member] = location

			continue
		}

		setDefault(compiled, location, uniform)

		if buffer != "cb_PdxCamera" && buffer != "cb_JominiEnvironment" && uniform.Kind == shader.KindFloat && uniform.Columns == 1 {
			built.shared[member] = sharedConstant{location: location, uniform: uniform}
		}
	}

	if state := program.States["RasterizerState"]; state != nil && strings.EqualFold(state.Values["CullMode"], "none") {
		built.twoSided = true
	}

	built.slope, built.steps = rasterizerBias(program.States["RasterizerState"])

	built.blend = blend

	if state := program.States["DepthStencilState"]; decal || state != nil && strings.EqualFold(state.Values["DepthWriteEnable"], "no") {
		built.noDepth = true
	}

	return built, nil
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

// opaqueOutput has the colour a pixel program writes come out opaque.
//
// What an effect without blending writes as alpha the games never show: it
// replaces what is drawn behind it, alpha or not, and some write whatever
// their diffuse map holds there, such as Crusader Kings 3's buildings, whose
// atlas holds 0. The picture of a viewer keeps its alpha, to be laid over
// what is behind it, so such an effect would leave a hole. The program's own
// main is kept and called; the colour it writes is made opaque after.
func opaqueOutput(glsl string) string {
	match := colorOutput.FindStringSubmatch(glsl)
	if match == nil || !strings.Contains(glsl, "void main()") {
		return glsl
	}

	glsl = strings.Replace(glsl, "void main()", "void pdx_main()", 1)

	return glsl + `
void main()
{
    pdx_main();
    ` + match[1] + `.a = 1.0;
}
`
}

// stateIsSet reports whether a state sets a value to yes, whatever case the
// file writes its name in.
func stateIsSet(state *shader.State, name string) bool {
	for key, value := range state.Values {
		if strings.EqualFold(key, name) && strings.EqualFold(value, "yes") {
			return true
		}
	}

	return false
}

// coverageOutput has a pixel program draw only where the alpha it writes
// covers at least half of the pixel.
//
// An effect that sets alpha to coverage, such as the leaves of Victoria 3's
// trees, covers as many of the samples of a pixel as its alpha says, which
// is how the soft edges of its leaves thin out. A picture of one sample a
// pixel has that sample covered or not; the program's own main is kept and
// called, and the pixel dropped after when its alpha is below a half.
func coverageOutput(glsl string) string {
	match := colorOutput.FindStringSubmatch(glsl)
	if match == nil || !strings.Contains(glsl, "void main()") {
		return glsl
	}

	glsl = strings.Replace(glsl, "void main()", "void pdx_coverage_main()", 1)

	return glsl + `
void main()
{
    pdx_coverage_main();
    if (` + match[1] + `.a < 0.5)
    {
        discard;
    }
}
`
}

// markNonFinite has a pixel program write magenta where the colour it
// writes is not a number or infinite, which a debugging session tells apart
// from black that way. The bits are compared, since a driver may take
// isnan to be false.
func markNonFinite(glsl string) string {
	match := colorOutput.FindStringSubmatch(glsl)
	if match == nil || !strings.Contains(glsl, "void main()") {
		return glsl
	}

	glsl = strings.Replace(glsl, "void main()", "void pdx_marked_main()", 1)

	return glsl + `
void main()
{
    pdx_marked_main();
    uvec4 Bits = floatBitsToUint(` + match[1] + `) & uvec4(0x7f800000u);
    if (any(equal(Bits.rgb, uvec3(0x7f800000u))))
    {
        ` + match[1] + ` = vec4(1.0, 0.0, 1.0, 1.0);
    }
}
`
}

// colorOutput finds the colour a pixel program writes to the picture.
var colorOutput = regexp.MustCompile(`layout\(location = 0\) out vec4 (\w+);`)

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

// drawEffect draws the pieces of a part with an effect: its own, or the one
// it casts its shadow with.
func (m *Model) drawEffect(part *gpuPart, compiled *effect, transform rl.Matrix) {
	current := m.renderer.frame

	// What raylib has batched is drawn first, so that it is not drawn with
	// this effect. The blending is set before the effect is: raylib draws
	// its batch as the blending changes, with its own shader, and leaves no
	// shader bound after.
	rl.DrawRenderBatchActive()

	if compiled.blend {
		blendOver()
	}

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

	m.renderer.applyEnvironment(compiled)
	m.renderer.applyShadows(compiled)

	for _, bound := range compiled.textures {
		rl.ActiveTextureSlot(bound.unit)

		srgb := false

		switch {
		case bound.texture.Type == "sampler2DShadow":
			rl.EnableTexture(m.renderer.shadowTexture().ID)
		case strings.HasPrefix(bound.texture.Type, "samplerCube"):
			var cube rl.Texture2D
			cube, srgb = m.renderer.cubeFor(bound.texture)
			rl.EnableTextureCubemap(cube.ID)
		case bound.texture.Type == "sampler2D":
			var flat rl.Texture2D
			flat, srgb = part.textureFor(bound.texture)
			rl.EnableTexture(flat.ID)
		}

		if bound.srgb >= 0 {
			on := uint32(0)
			if srgb {
				on = 1
			}

			rl.SetShaderValue(compiled.shader, bound.srgb, []float32{math.Float32frombits(on)}, rl.ShaderUniformInt)
		}
	}

	if compiled.twoSided {
		rl.DisableBackfaceCulling()
	}

	if compiled.noDepth {
		rl.DisableDepthMask()
	}

	if compiled.slope != 0 || compiled.steps != 0 {
		depthBias(compiled.slope, compiled.steps)
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

	if compiled.slope != 0 || compiled.steps != 0 {
		depthBias(0, 0)
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

	if compiled.blend {
		rl.SetBlendMode(rl.BlendAlpha)
	}
}

// textureFor is the texture a sampler of the part's effect reads, and
// whether it holds colours: the asset's texture of the sampler's slot, the
// file the sampler names when the asset gives none, or what stands in for
// what the engine binds.
func (p *gpuPart) textureFor(texture shader.Texture) (rl.Texture2D, bool) {
	if texture.Sampler != nil {
		if slot, ok := samplerSlot(texture.Sampler); ok {
			if found, ok := p.slots[slot]; ok {
				return found, p.srgb[slot]
			}
		}

		if found, ok := p.files[texture.Sampler.File]; ok && texture.Sampler.File != "" {
			return found, texture.Sampler.SRGB
		}
	}

	if found, ok := p.standIns[texture.Name]; ok {
		return found.texture, found.srgb
	}

	return p.white, false
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
		// A file that could not be read stands in as the white pixel or the
		// white cube, which belong to the renderer.
		if file.ID != r.white.ID && file.ID != r.whiteCube.ID && file.ID != r.none.ID && file.ID != r.grey.ID {
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
