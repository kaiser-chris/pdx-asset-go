package render

import (
	"math"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/shader"
)

// The shadows of the sun.
//
// The games draw what casts a shadow into a shadow map first, from the
// direction their environment casts shadows from, each part with the effect
// its settings cast its shadow with, such as standardShadow: depth only,
// offset by the bias of the effect's rasterizer state. Their effects then
// find a point in that map by ShadowMapTextureMatrix of PdxCamera and
// compare its depth with the map's through the sampler of PdxShadowmap, at
// a few places around it, which the constants of PdxShadowmap give.
//
// The map here covers the models a viewer draws: the sphere around the box
// they fit in, seen from the direction of the shadows.

// shadowMapSize is how many pixels across the shadow map is.
const shadowMapSize = 2048

// shadowSamples is how many pairs of places around a point the effects
// compare, NumSamples of PdxShadowmap: all that DiscSamples holds.
const shadowSamples = 8

// discSamples are the places around a point the effects compare, two to an
// element, x and y then z and w, within about one of the point, which the effects
// turn by a random angle and scale by KernelScale: a Poisson disc of
// sixteen points, spread evenly without a pattern.
var discSamples = [shadowSamples][4]float32{
	{-0.94201624, -0.39906216, 0.94558609, -0.76890725},
	{-0.094184101, -0.92938870, 0.34495938, 0.29387760},
	{-0.91588581, 0.45771432, -0.81544232, -0.87912464},
	{-0.38277543, 0.27676845, 0.97484398, 0.75648379},
	{0.44323325, -0.97511554, 0.53742981, -0.47373420},
	{-0.26496911, -0.41893023, 0.79197514, 0.19090188},
	{-0.24188840, 0.99706507, -0.81409955, 0.91437590},
	{0.19984126, 0.78641367, 0.14383161, -0.14100790},
}

// shadowMap is the shadow map of the frame being drawn.
type shadowMap struct {
	framebuffer uint32
	depth       rl.Texture2D

	// view and projection are the camera of the sun the map was drawn
	// from, and cast says the map holds the shadows of the frame.
	view, projection rl.Matrix
	cast             bool
}

// castShadows draws the shadows of models into the shadow map, for the
// effects to compare with as the models are drawn next. Without an
// environment, there is no sun to cast them, and with nothing in the
// models that casts one, nothing is drawn.
func (r *Renderer) castShadows(models []*Model) {
	r.shadows.cast = false

	if r.environment == nil || !castsShadows(models) {
		return
	}

	if r.shadows.framebuffer == 0 && !r.prepareShadows() {
		return
	}

	low, high, ok := boundsOf(models)
	if !ok {
		return
	}

	sun := shadowCamera(low, high, shadowLight(r.environment.ShadowDirection()))
	r.shadows.view, r.shadows.projection = sun.view, sun.projection

	// What raylib has batched belongs to the picture, not the map.
	rl.DrawRenderBatchActive()

	rl.EnableFramebuffer(r.shadows.framebuffer)
	rl.Viewport(0, 0, shadowMapSize, shadowMapSize)
	rl.EnableDepthMask()
	rl.EnableDepthTest()
	rl.ClearScreenBuffers()

	drawn := r.frame
	r.frame = sun

	for _, model := range models {
		if model != nil {
			model.drawShadows(rl.MatrixIdentity())
		}
	}

	r.frame = drawn

	rl.DisableDepthTest()
	rl.DisableFramebuffer()

	r.shadows.cast = true
}

// shadowCamera is the camera of the sun a shadow map of what fits in a box
// is drawn from: looking along the light at the sphere around the box, all
// of which it sees, from twice its radius away.
func shadowCamera(low, high, toLight rl.Vector3) frame {
	centre := rl.Vector3Scale(rl.Vector3Add(low, high), 0.5)
	radius := max(rl.Vector3Distance(low, high)/2, 1e-3)

	up := rl.Vector3{Y: 1}
	if math.Abs(float64(toLight.Y)) > 0.99 {
		up = rl.Vector3{Z: 1}
	}

	eye := rl.Vector3Add(centre, rl.Vector3Scale(toLight, 2*radius))

	return frame{
		view:       rl.MatrixLookAt(eye, centre, up),
		projection: rl.MatrixOrtho(-radius, radius, -radius, radius, radius, 3*radius),
		camera:     rl.Camera3D{Position: eye, Target: centre, Up: up},
		near:       radius,
		far:        3 * radius,
	}
}

// shadowLight is the direction towards the light the shadows are cast from,
// in OpenGL's coordinates, of a direction of the game's.
//
// Unlike the sun, which lights the models mirrored across x along with
// them, the shadows fall as the direction says without the mirror: so
// Victoria 3's model editor casts them by its environment file, which
// offsets them far to the side of the sun. Its pictures of
// french_city_academy_01_entity show the shadow of the wing on the right
// across the courtyard and the inner court to the right of the dome dark,
// where the mirrored direction has the left wing shade the courtyard
// instead, while the sun lights the dome from the side it does in the game
// only mirrored.
func shadowLight(direction [3]float32) rl.Vector3 {
	return rl.Vector3{X: direction[0], Y: direction[1], Z: direction[2]}
}

// castsShadows reports whether any part of the models casts a shadow.
func castsShadows(models []*Model) bool {
	for _, model := range models {
		if model == nil {
			continue
		}

		for _, part := range model.parts {
			if part.shadow != nil {
				return true
			}
		}
	}

	return false
}

// boundsOf is the box the models fit in.
func boundsOf(models []*Model) (low, high rl.Vector3, ok bool) {
	for _, model := range models {
		if model == nil || len(model.parts) == 0 {
			continue
		}

		lower := rl.Vector3{X: model.Min[0], Y: model.Min[1], Z: model.Min[2]}
		higher := rl.Vector3{X: model.Max[0], Y: model.Max[1], Z: model.Max[2]}

		if !ok {
			low, high, ok = lower, higher, true

			continue
		}

		low, high = rl.Vector3Min(low, lower), rl.Vector3Max(high, higher)
	}

	return low, high, ok
}

// prepareShadows makes the framebuffer the shadow map is drawn into.
func (r *Renderer) prepareShadows() bool {
	framebuffer := rl.LoadFramebuffer()
	if framebuffer == 0 {
		return false
	}

	depth := shadowDepth(shadowMapSize)
	rl.FramebufferAttach(framebuffer, depth.ID, rl.AttachmentDepth, rl.AttachmentTexture2d, 0)

	rl.EnableFramebuffer(framebuffer)
	noColor()
	rl.DisableFramebuffer()

	if !rl.FramebufferComplete(framebuffer) {
		// Unloading the framebuffer unloads the texture attached to it.
		rl.UnloadFramebuffer(framebuffer)

		return false
	}

	r.shadows.framebuffer, r.shadows.depth = framebuffer, depth

	return true
}

// releaseShadows unloads the shadow map.
func (r *Renderer) releaseShadows() {
	if r.shadows.framebuffer != 0 {
		// The depth texture goes with it.
		rl.UnloadFramebuffer(r.shadows.framebuffer)
	}

	r.shadows = shadowMap{}
}

// shadowTexture is the shadow map an effect's comparison sampler reads: the
// one of the frame once it is cast, and one that is lit everywhere before
// and without.
func (r *Renderer) shadowTexture() rl.Texture2D {
	if r.shadows.cast {
		return r.shadows.depth
	}

	return r.shadowMap
}

// shadowTextureMatrix takes a point of the world to where it is in the
// shadow map and how deep, each from 0 to 1, as ShadowMapTextureMatrix
// does.
func (r *Renderer) shadowTextureMatrix() rl.Matrix {
	toTexture := rl.MatrixMultiply(rl.MatrixScale(0.5, 0.5, 0.5), rl.MatrixTranslate(0.5, 0.5, 0.5))

	return rl.MatrixMultiply(rl.MatrixMultiply(r.shadows.view, r.shadows.projection), toTexture)
}

// applyShadows sets the constants of PdxShadowmap and ShadowMapTextureMatrix
// on an effect: those of the shadow map once it is cast, and before or
// without it a single comparison that is lit everywhere, rather than none,
// which the effects divide the sum of their comparisons by.
func (r *Renderer) applyShadows(compiled *effect) {
	set := func(name string, value float32) {
		if location, ok := compiled.shadow[name]; ok {
			rl.SetShaderValue(compiled.shader, location, []float32{value}, rl.ShaderUniformFloat)
		}
	}

	setInt := func(name string, value uint32) {
		if location, ok := compiled.shadow[name]; ok {
			rl.SetShaderValue(compiled.shader, location, []float32{math.Float32frombits(value)}, rl.ShaderUniformInt)
		}
	}

	if !r.shadows.cast {
		setInt("NumSamples", 1)
		set("ShadowFadeFactor", 0)

		return
	}

	if location, ok := compiled.camera["ShadowMapTextureMatrix"]; ok {
		rl.SetShaderValueMatrix(compiled.shader, location, rl.MatrixTranspose(r.shadowTextureMatrix()))
	}

	shadow := r.environment.Shadow

	setInt("NumSamples", shadowSamples)
	set("ShadowFadeFactor", 1)
	set("Bias", shadow.DepthBias)
	set("KernelScale", shadow.KernelScale/shadowMapSize)
	set("ShadowScreenSpaceScale", shadowMapSize)

	if location, ok := compiled.shadow["DiscSamples[0]"]; ok {
		values := make([]float32, 0, 4*shadowSamples)
		for _, sample := range discSamples {
			values = append(values, sample[:]...)
		}

		rl.SetShaderValueV(compiled.shader, location, values, rl.ShaderUniformVec4, shadowSamples)
	}
}

// rasterizerBias is the bias a rasterizer state offsets depth by: its
// SlopeScaleDepthBias and its DepthBias, in steps of the depth buffer.
func rasterizerBias(state *shader.State) (slope, steps float32) {
	if state == nil {
		return 0, 0
	}

	for key, value := range state.Values {
		number, err := strconv.ParseFloat(value, 32)
		if err != nil {
			continue
		}

		switch {
		case strings.EqualFold(key, "SlopeScaleDepthBias"):
			slope = float32(number)
		case strings.EqualFold(key, "DepthBias"):
			steps = float32(number)
		}
	}

	return slope, steps
}
