package render

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// The post effect.
//
// The games light their models in linear light whose brightness has no
// upper bound, and turn the lit picture into what the screen shows in a pass
// of their own: Jomini's restorescene.shader, which applies the exposure and
// the tonemap curve the environment file picks, hue, saturation and value,
// colour balance and levels, and converts to gamma. A viewer that shows the
// lit picture as it is clips every colour brighter than white and shows the
// rest without the curve or the gamma: washed out.
//
// The viewer draws its models into a picture of half floats and runs the
// game's own pass over it, the effect RestoreAlpha, which keeps the alpha
// the picture is laid over its background by. What the pass offers beyond,
// bloom, depth of field, lens flares and colour grading by a table, it is
// compiled without.
//
// The pass draws a quad that fills the picture. Its vertex stage is the
// renderer's own: the game's reads the corners as integers, which raylib
// does not hand over, and does nothing else but pass on where in the
// picture each corner is, which the renderer's does too.

// restoreFile and restoreEffect are the game's pass.
const (
	restoreFile   = "gfx/FX/jomini/restorescene.shader"
	restoreEffect = "RestoreAlpha"
)

// tonemaps are the tonemaps an environment file names, by their name there
// in lower case: the define that picks one in Victoria 3's Jomini, and its
// number, which picks one in Crusader Kings 3's through TonemapIndex.
var tonemaps = map[string]struct {
	define string
	index  int
}{
	"none":                 {"TONEMAP_NONE", 0},
	"reinhard":             {"TONEMAP_REINHARD", 1},
	"reinhardmodified":     {"TONEMAP_REINHARD_MODIFIED", 2},
	"filmic_hable":         {"TONEMAP_FILMIC_HABLE", 3},
	"filmicaces_narkowicz": {"TONEMAP_FILMICACES_NARKOWICZ", 4},
	"filmicaces_hill":      {"TONEMAP_FILMICACES_HILL", 5},
	"uncharted":            {"TONEMAP_UNCHARTED", 6},
	"tonymcmapface":        {"TONEMAP_TONY_MCMAPFACE", 7},
	"uchimura":             {"TONEMAP_UCHIMURA", 8},
	"agx":                  {"TONEMAP_AGX", 9},
}

// exposures are the exposure functions, by their name in lower case, and the
// define that picks one. Only the fixed one can be had without the average
// brightness of the picture, which the others read from a texture the engine
// keeps; those leave the picture as bright as it is.
var exposures = map[string]string{
	"fixedexposure": "EXPOSURE_FIXED",
}

// postEffect is the game's pass, compiled.
type postEffect struct {
	shader   rl.Shader
	textures []effectTexture

	// screen and inverse are where the size of the picture goes, and one
	// over it, or -1.
	screen, inverse int32
}

// varying finds what the pixel stage of a pass reads from its vertex stage.
var varying = regexp.MustCompile(`\bin\s+(vec[234]|float)\s+(pdx_varying_\d+)\s*;`)

// post compiles the post effect for the environment once, or returns why it
// cannot be. Without the games' shaders there is none.
func (r *Renderer) post() (*postEffect, error) {
	if r.shaders == nil {
		return nil, nil
	}

	if r.postCompiled != nil || r.postErr != nil {
		return r.postCompiled, r.postErr
	}

	r.postCompiled, r.postErr = r.compilePost()

	return r.postCompiled, r.postErr
}

func (r *Renderer) compilePost() (*postEffect, error) {
	settings := environment.Default().Post
	if r.environment != nil {
		settings = r.environment.Post
	}

	var defines []string
	if define, ok := exposures[strings.ToLower(settings.Exposure)]; ok {
		defines = append(defines, define)
	}

	tonemap, ok := tonemaps[strings.ToLower(settings.Tonemap)]
	if !ok {
		tonemap = tonemaps["uncharted"]
	}

	program, err := r.shaders.Program(restoreFile, restoreEffect, defines)
	if err != nil {
		return nil, fmt.Errorf("the post effect: %w", err)
	}

	// The older Jomini picks its tonemap by a define, the newer by a
	// constant.
	byIndex := false
	for _, uniform := range program.Uniforms {
		if member(uniform.Name) == "TonemapIndex" {
			byIndex = true
		}
	}

	if !byIndex {
		program, err = r.shaders.Program(restoreFile, restoreEffect, append(defines, tonemap.define))
		if err != nil {
			return nil, fmt.Errorf("the post effect: %w", err)
		}
	}

	compiled := rl.LoadShaderFromMemory(postVertex(program.Pixel), program.Pixel)
	if compiled.ID == 0 || compiled.ID == rl.GetShaderIdDefault() {
		return nil, fmt.Errorf("the post effect: the driver did not take the program")
	}

	built := &postEffect{shader: compiled, screen: -1, inverse: -1}

	// The picture is the texture raylib draws the quad with, on unit 0;
	// the other samplers, such as the table of Crusader Kings 3's tonemap,
	// take units of their own.
	unit := int32(1)

	for _, texture := range program.Textures {
		location := rl.GetShaderLocation(compiled, texture.Name)
		if location < 0 {
			continue
		}

		bound := effectTexture{location: location, texture: texture}

		if texture.Sampler != nil && texture.Sampler.HasIndex && texture.Sampler.Index == 0 && texture.Sampler.Ref == "" {
			bound.unit = 0
		} else {
			bound.unit = unit
			unit++
			built.textures = append(built.textures, bound)
		}

		rl.SetShaderValue(compiled, location, []float32{math.Float32frombits(uint32(bound.unit))}, rl.ShaderUniformInt)
	}

	for _, uniform := range program.Uniforms {
		location := rl.GetShaderLocation(compiled, uniform.Name)
		if location < 0 {
			continue
		}

		setDefault(compiled, location, uniform)

		name := member(uniform.Name)

		switch name {
		case "ScreenResolution":
			built.screen = location
		case "InvScreenResolution":
			built.inverse = location
		}

		if name == "TonemapIndex" {
			rl.SetShaderValue(compiled, location, []float32{float32(tonemap.index) / 255}, rl.ShaderUniformFloat)

			continue
		}

		if values, ok := settings.Constants[name]; ok {
			setFloats(compiled, location, values)
		}
	}

	return built, nil
}

// member is the name of a constant within its buffer.
func member(uniform string) string {
	if _, name, found := strings.Cut(uniform, "."); found {
		return name
	}

	return uniform
}

// setFloats sets a constant of one to four numbers.
func setFloats(target rl.Shader, location int32, values []float32) {
	kinds := map[int]rl.ShaderUniformDataType{
		1: rl.ShaderUniformFloat,
		2: rl.ShaderUniformVec2,
		3: rl.ShaderUniformVec3,
		4: rl.ShaderUniformVec4,
	}

	if kind, ok := kinds[len(values)]; ok {
		rl.SetShaderValue(target, location, values, kind)
	}
}

// postVertex is the vertex stage of the pass: raylib's quad, passing on
// where in the picture each corner is to every input of the pixel stage.
func postVertex(pixel string) string {
	var out strings.Builder

	out.WriteString("#version 330\n\nin vec3 vertexPosition;\nin vec2 vertexTexCoord;\nuniform mat4 mvp;\n\n")

	var assignments []string

	for _, match := range varying.FindAllStringSubmatch(pixel, -1) {
		fmt.Fprintf(&out, "out %s %s;\n", match[1], match[2])

		switch match[1] {
		case "float":
			assignments = append(assignments, match[2]+" = vertexTexCoord.x;")
		case "vec2":
			assignments = append(assignments, match[2]+" = vertexTexCoord;")
		case "vec3":
			assignments = append(assignments, match[2]+" = vec3(vertexTexCoord, 0.0);")
		case "vec4":
			assignments = append(assignments, match[2]+" = vec4(vertexTexCoord, 0.0, 1.0);")
		}
	}

	out.WriteString("\nvoid main()\n{\n")

	for _, assignment := range assignments {
		out.WriteString("    " + assignment + "\n")
	}

	out.WriteString("    gl_Position = mvp * vec4(vertexPosition, 1.0);\n}\n")

	return out.String()
}

// drawPost draws a picture of the lit models through the post effect into
// the current target.
func (r *Renderer) drawPost(compiled *postEffect, scene rl.Texture2D) {
	// The picture is laid over the background by its alpha. The blending is
	// set first: raylib draws its batch as it changes.
	blendOver()

	rl.BeginShaderMode(compiled.shader)

	width, height := float32(scene.Width), float32(scene.Height)
	if compiled.screen >= 0 {
		rl.SetShaderValue(compiled.shader, compiled.screen, []float32{width, height}, rl.ShaderUniformVec2)
	}

	if compiled.inverse >= 0 {
		rl.SetShaderValue(compiled.shader, compiled.inverse, []float32{1 / width, 1 / height}, rl.ShaderUniformVec2)
	}

	for _, bound := range compiled.textures {
		rl.ActiveTextureSlot(bound.unit)

		if bound.texture.Sampler != nil && bound.texture.Sampler.File != "" {
			rl.EnableTexture(r.fileTexture(bound.texture.Sampler.File).ID)
		} else {
			rl.EnableTexture(r.white.ID)
		}
	}

	rl.ActiveTextureSlot(0)

	// A render target is filled bottom up, so it is read upside down.
	rl.DrawTextureRec(scene,
		rl.Rectangle{Width: float32(scene.Width), Height: -float32(scene.Height)},
		rl.Vector2{}, rl.White)

	rl.EndShaderMode()
	rl.SetBlendMode(rl.BlendAlpha)

	for _, bound := range compiled.textures {
		rl.ActiveTextureSlot(bound.unit)
		rl.DisableTexture()
	}

	rl.ActiveTextureSlot(0)
}

// releasePost unloads the compiled post effect, for the next environment or
// shaders.
func (r *Renderer) releasePost() {
	if r.postCompiled != nil {
		rl.UnloadShader(r.postCompiled.shader)
	}

	r.postCompiled, r.postErr = nil, nil
}

// hdrTarget is a picture of half floats with a depth buffer, which keeps the
// colours brighter than white that lit models have.
func hdrTarget(width, height int32) (rl.RenderTexture2D, error) {
	framebuffer := rl.LoadFramebuffer()
	if framebuffer == 0 {
		return rl.RenderTexture2D{}, fmt.Errorf("no framebuffer for the lit picture")
	}

	color := rl.LoadTextureFromImage(&rl.Image{Width: width, Height: height, Mipmaps: 1, Format: pixelFormats[texture.RGBA16F]})
	depth := rl.LoadTextureDepth(width, height, true)

	rl.FramebufferAttach(framebuffer, color.ID, rl.AttachmentColorChannel0, rl.AttachmentTexture2d, 0)
	rl.FramebufferAttach(framebuffer, depth, rl.AttachmentDepth, rl.AttachmentRenderbuffer, 0)

	target := rl.RenderTexture2D{ID: framebuffer, Texture: color, Depth: rl.Texture2D{ID: depth, Width: width, Height: height}}

	if !rl.FramebufferComplete(framebuffer) {
		rl.UnloadRenderTexture(target)

		return rl.RenderTexture2D{}, fmt.Errorf("the framebuffer for the lit picture is incomplete")
	}

	return target, nil
}
