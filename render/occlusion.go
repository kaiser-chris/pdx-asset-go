package render

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The games darken a portrait where its surfaces are hidden from the light
// around them, in the creases of the eyes, the nose and the mouth and where
// the head meets the neck: screen space ambient occlusion, worked out from the
// depth of the picture as jomini's ssao.shader does, blurred along the
// surfaces, and laid over the light as its ssao_apply.shader does. Where they
// darken, they darken towards a colour each part gives: for skin the deep red
// of its ssao_color texture, multiplied by the skin colour, which keeps the
// shadowed skin warm; for clothes black.
//
// The viewer draws the depth of the models from its camera first, works the
// occlusion out from it into a picture of its own and blurs it, and the
// shader looks each pixel up in that as it lights the pixel, before the light
// is tonemapped, which is when the games lay it over theirs.

// Occlusion is how the ambient occlusion of a Lighting is worked out and laid
// over it, as the games' ssao settings of jomini_environment.txt write it.
type Occlusion struct {
	// Samples is how many points around a pixel its depth is compared with,
	// at most maxOcclusionSamples. Radius is how far around it they are, as
	// a part of the picture at a depth of one, and MaxRadius the most of the
	// picture that is, however near.
	Samples           int
	Radius, MaxRadius float32

	// DepthDistance is how much nearer a point has to be than the pixel to
	// hide it fully, and MaxDepth how much nearer it can be and still hide
	// it, rather than stand in front of it.
	DepthDistance, MaxDepth float32

	// Scale, Bias and Power shape how much light is left, as the games'
	// apply_scale, apply_bias and apply_power, and BlendFactor is how much of
	// what is hidden is darkened.
	Scale, Bias, Power float32
	BlendFactor        float32

	// Blur blurs the occlusion along the surfaces, over BlurWeights pixels
	// each way, spread as widely as BlurWidth, a BlurScale of pixels apart,
	// and held back by a difference in depth as strongly as BlurDepthPower.
	Blur                                 bool
	BlurWidth, BlurScale, BlurDepthPower float32
	BlurWeights                          int
}

// maxOcclusionSamples is how many samples the occlusion can take, as many as
// the games' shader has room for.
const maxOcclusionSamples = 16

// maxBlurWeights is how many pixels the blur can take each way, as many as the
// games' shader has room for, with the one in the middle.
const maxBlurWeights = 5

// The texture units the occlusion and the colour a part is darkened towards
// are bound to, beside the shadow map.
const (
	occlusionUnit       = shadowUnit + 1
	occlusionColourUnit = shadowUnit + 2
)

// discSamples are the points around a pixel the occlusion compares its depth
// with: spread evenly over a disc of radius one, each with the height of the
// sphere over it at that point. The games hand theirs over from the engine,
// which does not ship them, so these are a spiral of Vogel's, which covers
// the disc as evenly as any count of points can.
func discSamples(count int) []float32 {
	samples := make([]float32, 0, maxOcclusionSamples*3)
	golden := math.Pi * (3 - math.Sqrt(5))

	for index := range maxOcclusionSamples {
		radius := math.Sqrt((float64(index) + 0.5) / float64(max(count, 1)))
		radius = math.Min(radius, 1)
		angle := float64(index) * golden

		samples = append(samples,
			float32(radius*math.Cos(angle)),
			float32(radius*math.Sin(angle)),
			float32(math.Sqrt(1-radius*radius)))
	}

	return samples
}

// blurWeights are the weights of the pixels the blur takes, the one in the
// middle first, of a normal distribution as wide as the width. The games
// work theirs out in the engine, which does not ship how.
func blurWeights(count int, width float32) []float32 {
	weights := make([]float32, maxBlurWeights)
	spread := math.Max(float64(width), 0.5)

	for index := range min(max(count, 1), maxBlurWeights) {
		weights[index] = float32(math.Exp(-float64(index*index) / (2 * spread * spread)))
	}

	return weights
}

// occlusionShader works the occlusion out and blurs it.
type occlusionShader struct {
	shader rl.Shader

	pass, resolution, planes  int32
	depthMap, blurSource      int32
	shape, apply, discSamples int32
	blur, blurWeights         int32
}

// loadOcclusionShader compiles the shader the occlusion is worked out with.
func loadOcclusionShader() (occlusionShader, error) {
	fragment, err := shaders.ReadFile("shaders/occlusion.fs")
	if err != nil {
		return occlusionShader{}, err
	}

	shader := rl.LoadShaderFromMemory("", string(fragment))
	if shader.ID == 0 || shader.ID == rl.GetShaderIdDefault() {
		return occlusionShader{}, fmt.Errorf("the occlusion shader did not compile")
	}

	location := func(name string) int32 { return rl.GetShaderLocation(shader, name) }

	return occlusionShader{
		shader:      shader,
		pass:        location("pass"),
		resolution:  location("resolution"),
		planes:      location("planes"),
		depthMap:    location("depthMap"),
		blurSource:  location("blurSource"),
		shape:       location("shape"),
		apply:       location("apply"),
		discSamples: location("discSamples"),
		blur:        location("blur"),
		blurWeights: location("blurWeights"),
	}, nil
}

// unload releases the shader.
func (s *occlusionShader) unload() {
	if s.shader.ID != 0 {
		rl.UnloadShader(s.shader)
		s.shader = rl.Shader{}
	}
}

// occlusionTarget is what a viewer keeps of the occlusion: the depth its
// camera sees and the two pictures the occlusion is worked out and blurred
// between, all the size of the viewer's picture.
type occlusionTarget struct {
	depth rl.RenderTexture2D
	work  [2]rl.RenderTexture2D
}

// prepare makes the targets in a size, or again in a new one, and reports
// whether there are any.
func (o *occlusionTarget) prepare(width, height int32) bool {
	if o.depth.ID != 0 && o.depth.Texture.Width == width && o.depth.Texture.Height == height {
		return true
	}

	o.unload()

	framebuffer := rl.LoadFramebuffer()
	if framebuffer == 0 {
		return false
	}

	depth := rl.LoadTextureDepth(width, height, false)
	rl.EnableFramebuffer(framebuffer)
	rl.FramebufferAttach(framebuffer, depth, attachmentDepth, attachmentTexture2D, 0)
	complete := rl.FramebufferComplete(framebuffer)
	rl.DisableFramebuffer()

	if !complete {
		rl.UnloadTexture(rl.Texture2D{ID: depth})
		rl.UnloadFramebuffer(framebuffer)

		return false
	}

	// BeginTextureMode takes the size to draw at from the colour texture,
	// which a target of depth alone has none of but its size.
	o.depth = rl.RenderTexture2D{
		ID:      framebuffer,
		Texture: rl.Texture2D{Width: width, Height: height},
		Depth:   rl.Texture2D{ID: depth, Width: width, Height: height, Mipmaps: 1},
	}

	for index := range o.work {
		work, ok := floatTarget(width, height)
		if !ok {
			o.unload()

			return false
		}

		o.work[index] = work
	}

	return true
}

// floatTarget is a picture of one channel of floating point numbers to draw
// into: the light left over a convex surface is above one, which the blur
// takes as it is, so that it makes up for the creases near it as the games'
// does, rather than cut off at one.
func floatTarget(width, height int32) (rl.RenderTexture2D, bool) {
	framebuffer := rl.LoadFramebuffer()
	if framebuffer == 0 {
		return rl.RenderTexture2D{}, false
	}

	blank := rl.GenImageColor(int(width), int(height), rl.White)
	rl.ImageFormat(blank, rl.UncompressedR32)
	colour := rl.LoadTextureFromImage(blank)
	rl.UnloadImage(blank)

	rl.EnableFramebuffer(framebuffer)
	rl.FramebufferAttach(framebuffer, colour.ID, attachmentColour, attachmentTexture2D, 0)
	complete := colour.ID != 0 && rl.FramebufferComplete(framebuffer)
	rl.DisableFramebuffer()

	if !complete {
		rl.UnloadTexture(colour)
		rl.UnloadFramebuffer(framebuffer)

		return rl.RenderTexture2D{}, false
	}

	return rl.RenderTexture2D{ID: framebuffer, Texture: colour}, true
}

// attachmentColour is the constant rlgl attaches the first colour texture
// with, from rlgl.h.
const attachmentColour = 0

// unload releases the targets. It needs the OpenGL context.
func (o *occlusionTarget) unload() {
	if o.depth.ID != 0 {
		rl.UnloadTexture(o.depth.Depth)
		rl.UnloadFramebuffer(o.depth.ID)
		o.depth = rl.RenderTexture2D{}
	}

	for index := range o.work {
		if o.work[index].ID != 0 {
			rl.UnloadTexture(o.work[index].Texture)
			rl.UnloadFramebuffer(o.work[index].ID)
			o.work[index] = rl.RenderTexture2D{}
		}
	}
}

// draw draws the depth of the models from a camera, between clip planes,
// works the occlusion out from it and blurs it, and returns the picture that
// holds it, in its red, or false where there is none.
func (o *occlusionTarget) draw(renderer *Renderer, occlusion *Occlusion, models []*Model, camera rl.Camera3D, near, far float64, width, height int32) (rl.Texture2D, bool) {
	if renderer.occlusion.shader.ID == 0 || !o.prepare(width, height) {
		return rl.Texture2D{}, false
	}

	// The models are drawn as they always are, which the depth is all that
	// is kept of, and without the occlusion that is being worked out.
	rl.SetShaderValue(renderer.shader, renderer.occluded, []float32{0}, rl.ShaderUniformFloat)
	rl.SetShaderValue(renderer.shader, renderer.lit.shadowed, []float32{0}, rl.ShaderUniformFloat)

	rl.BeginTextureMode(o.depth)
	rl.ClearBackground(rl.White)
	rl.SetClipPlanes(near, far)
	rl.BeginMode3D(camera)

	for _, drawn := range models {
		if drawn != nil {
			drawn.Draw(rl.MatrixIdentity())
		}
	}

	rl.EndMode3D()
	rl.EndTextureMode()

	s := &renderer.occlusion
	resolution := []float32{float32(width), float32(height)}

	rl.SetShaderValue(s.shader, s.resolution, resolution, rl.ShaderUniformVec2)
	rl.SetShaderValue(s.shader, s.planes, []float32{float32(near), float32(far)}, rl.ShaderUniformVec2)
	rl.SetShaderValue(s.shader, s.shape, []float32{occlusion.Radius, occlusion.MaxRadius, max(occlusion.DepthDistance, 1e-3), max(occlusion.MaxDepth, 1e-3)}, rl.ShaderUniformVec4)

	samples := min(max(occlusion.Samples, 1), maxOcclusionSamples)
	rl.SetShaderValue(s.shader, s.apply, []float32{occlusion.Scale, occlusion.Bias, occlusion.Power, float32(samples)}, rl.ShaderUniformVec4)
	rl.SetShaderValueV(s.shader, s.discSamples, discSamples(samples), rl.ShaderUniformVec3, maxOcclusionSamples)

	o.pass(s, 0, rl.Texture2D{}, 0)

	if !occlusion.Blur || occlusion.BlurWeights < 2 {
		return o.work[0].Texture, true
	}

	count := min(occlusion.BlurWeights, maxBlurWeights)
	rl.SetShaderValueV(s.shader, s.blurWeights, blurWeights(count, occlusion.BlurWidth), rl.ShaderUniformFloat, maxBlurWeights)
	rl.SetShaderValue(s.shader, s.blur, []float32{max(occlusion.BlurScale, 1), occlusion.BlurDepthPower, float32(count), 0}, rl.ShaderUniformVec4)

	// Across, from the first picture into the second, and then down, back
	// into the first.
	o.pass(s, 1, o.work[0].Texture, 0)
	o.pass(s, 1, o.work[1].Texture, 1)

	return o.work[0].Texture, true
}

// pass draws one pass of the occlusion shader over the whole of a picture:
// working the occlusion out into the first, or blurring a source along an
// axis, 0 across and 1 down, into the picture it is not.
func (o *occlusionTarget) pass(s *occlusionShader, kind int, source rl.Texture2D, axis int) {
	into := o.work[0]
	if kind == 1 && axis == 0 {
		into = o.work[1]
	}

	rl.BeginTextureMode(into)
	rl.ClearBackground(rl.White)
	rl.BeginShaderMode(s.shader)

	rl.SetShaderValue(s.shader, s.pass, []float32{float32(kind), float32(axis)}, rl.ShaderUniformVec2)

	rl.ActiveTextureSlot(occlusionUnit)
	rl.EnableTexture(o.depth.Depth.ID)
	rl.SetUniform(s.depthMap, []int32{occlusionUnit}, int32(rl.ShaderUniformInt), 1)

	if kind == 1 {
		rl.ActiveTextureSlot(occlusionColourUnit)
		rl.EnableTexture(source.ID)
		rl.SetUniform(s.blurSource, []int32{occlusionColourUnit}, int32(rl.ShaderUniformInt), 1)
	}

	rl.ActiveTextureSlot(0)

	// The shader finds what it reads by the place of the pixel it draws, so
	// any shape over the whole picture does.
	rl.DrawRectangle(0, 0, into.Texture.Width, into.Texture.Height, rl.White)

	rl.EndShaderMode()
	rl.EndTextureMode()
}

// applyOcclusion tells the shader where to look the occlusion up and how
// much of it to lay over the light, or that there is none.
func (r *Renderer) applyOcclusion(occlusion *Occlusion, picture rl.Texture2D, drawn bool, width, height int32) {
	if occlusion == nil || !drawn {
		rl.SetShaderValue(r.shader, r.occluded, []float32{0}, rl.ShaderUniformFloat)

		return
	}

	rl.SetShaderValue(r.shader, r.occluded, []float32{1}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.occlusionBlend, []float32{occlusion.BlendFactor}, rl.ShaderUniformFloat)
	rl.SetShaderValue(r.shader, r.occlusionResolution, []float32{float32(width), float32(height)}, rl.ShaderUniformVec2)

	rl.EnableShader(r.shader.ID)
	rl.ActiveTextureSlot(occlusionUnit)
	rl.EnableTexture(picture.ID)
	rl.SetUniform(r.occlusionMap, []int32{occlusionUnit}, int32(rl.ShaderUniformInt), 1)
	rl.SetUniform(r.occlusionColour, []int32{occlusionColourUnit}, int32(rl.ShaderUniformInt), 1)
	rl.ActiveTextureSlot(0)
}

// bindOcclusionColour binds the colour a part is darkened towards where it
// is hidden. It has to run while this renderer's shader is the one in use.
func (m *Model) bindOcclusionColour(part *gpuPart) {
	r := m.renderer

	rl.SetShaderValue(r.shader, r.occlusionTinted, []float32{boolFloat(part.look.palette)}, rl.ShaderUniformFloat)

	if part.occlusion.ID == 0 {
		return
	}

	rl.EnableShader(r.shader.ID)
	rl.ActiveTextureSlot(occlusionColourUnit)
	rl.EnableTexture(part.occlusion.ID)
	rl.ActiveTextureSlot(0)
}
