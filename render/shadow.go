package render

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The games cast the shadows of a portrait from a camera of their own, the
// shadow camera of an environment, and darken the light of every light marked
// as affected by shadow wherever that camera does not see the surface: a
// portrait lit from in front by a light behind it on one side has that
// light's sheen on its jaw taken away, since the jaw faces away from where the
// shadows are cast from. The viewer draws the depth of the models seen from
// that camera into a texture first, and the shader looks each pixel up in it.

// Shadow is where the shadows of a Lighting are cast from, in the
// coordinates the models are drawn in, and how they are drawn.
type Shadow struct {
	// Position is where the shadow camera stands and Target the point it
	// looks at. FieldOfView is how much it sees, top to bottom, in degrees,
	// and Near and Far how near and how far.
	Position, Target [3]float32
	FieldOfView      float32
	Near, Far        float32

	// KernelScale is how soft the edge of a shadow is, as the games'
	// shadowmap_kernelscale, and FadeFactor how much of a light a shadow
	// takes away, from 0 for none to 1 for all of it.
	KernelScale, FadeFactor float32
}

// shadowSize is how many texels a side the depth of the shadow camera is
// drawn at: enough for the edge of a portrait's shadow on itself, which is
// all the games cast.
const shadowSize = 2048

// shadowUnit is the texture unit the shadow map is bound to, beside the
// environment's.
const shadowUnit = environmentUnit + 1

// The constants rlgl attaches a depth texture with, from rlgl.h.
const (
	attachmentDepth     = 100
	attachmentTexture2D = 100
)

// shadowTarget is the depth the shadow camera sees, which a viewer keeps.
type shadowTarget struct {
	target rl.RenderTexture2D
}

// prepare makes the target, the first time it is wanted, and reports whether
// there is one.
func (s *shadowTarget) prepare() bool {
	if s.target.ID != 0 {
		return true
	}

	framebuffer := rl.LoadFramebuffer()
	if framebuffer == 0 {
		return false
	}

	depth := rl.LoadTextureDepth(shadowSize, shadowSize, false)
	rl.EnableFramebuffer(framebuffer)
	rl.FramebufferAttach(framebuffer, depth, attachmentDepth, attachmentTexture2D, 0)
	complete := rl.FramebufferComplete(framebuffer)
	rl.DisableFramebuffer()

	if !complete {
		rl.UnloadFramebuffer(framebuffer)

		return false
	}

	// BeginTextureMode takes the size to draw at from the colour texture,
	// which a target of depth alone has none of but its size.
	s.target = rl.RenderTexture2D{
		ID:      framebuffer,
		Texture: rl.Texture2D{Width: shadowSize, Height: shadowSize},
		Depth:   rl.Texture2D{ID: depth, Width: shadowSize, Height: shadowSize, Mipmaps: 1},
	}

	return true
}

// unload releases the target. It needs the OpenGL context.
func (s *shadowTarget) unload() {
	if s.target.ID == 0 {
		return
	}

	rl.UnloadTexture(s.target.Depth)
	rl.UnloadFramebuffer(s.target.ID)
	s.target = rl.RenderTexture2D{}
}

// draw draws the depth of models as the shadow camera sees them, and returns
// the matrix that takes a place of the models to where it lies in that
// picture.
func (s *shadowTarget) draw(renderer *Renderer, shadow *Shadow, models []*Model) (rl.Matrix, bool) {
	if !s.prepare() {
		return rl.Matrix{}, false
	}

	rl.BeginTextureMode(s.target)
	rl.ClearBackground(rl.White)

	near, far := float64(max(shadow.Near, 0.01)), float64(max(shadow.Far, shadow.Near+1))
	rl.SetClipPlanes(near, far)

	camera := rl.Camera3D{
		Position:   rl.Vector3{X: shadow.Position[0], Y: shadow.Position[1], Z: shadow.Position[2]},
		Target:     rl.Vector3{X: shadow.Target[0], Y: shadow.Target[1], Z: shadow.Target[2]},
		Up:         rl.Vector3{Y: 1},
		Fovy:       max(shadow.FieldOfView, 1),
		Projection: rl.CameraPerspective,
	}

	// The models are drawn as they always are, which the depth is all that
	// is kept of; the shadow is not looked up while it is being drawn.
	rl.SetShaderValue(renderer.shader, renderer.lit.shadowed, []float32{0}, rl.ShaderUniformFloat)

	rl.BeginMode3D(camera)

	view, projection := rl.GetMatrixModelview(), rl.GetMatrixProjection()

	renderer.castingShadows = true

	for _, drawn := range models {
		if drawn != nil {
			drawn.Draw(rl.MatrixIdentity())
		}
	}

	renderer.castingShadows = false

	rl.EndMode3D()
	rl.EndTextureMode()

	return rl.MatrixMultiply(view, projection), true
}

// applyShadow tells the shader where to look the shadow up, or that there is
// none.
func (r *Renderer) applyShadow(shadow *Shadow, target *shadowTarget, lightSpace rl.Matrix, drawn bool) {
	if shadow == nil || !drawn {
		rl.SetShaderValue(r.shader, r.lit.shadowed, []float32{0}, rl.ShaderUniformFloat)

		return
	}

	rl.SetShaderValue(r.shader, r.lit.shadowed, []float32{1}, rl.ShaderUniformFloat)
	rl.SetShaderValueMatrix(r.shader, r.lit.lightSpace, lightSpace)

	// The softness is spread over the texels of the picture: the games
	// write it for a picture of their own size, which is not shipped.
	softness := max(shadow.KernelScale, 1) / shadowSize * 0.5
	fade := shadow.FadeFactor
	if fade <= 0 {
		fade = 1
	}

	rl.SetShaderValue(r.shader, r.lit.shadowSoftness, []float32{softness, float32(math.Min(float64(fade), 1))}, rl.ShaderUniformVec2)

	rl.EnableShader(r.shader.ID)
	rl.ActiveTextureSlot(shadowUnit)
	rl.EnableTexture(target.target.Depth.ID)
	rl.SetUniform(r.lit.shadowMap, []int32{shadowUnit}, int32(rl.ShaderUniformInt), 1)
	rl.ActiveTextureSlot(0)
}
