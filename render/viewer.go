package render

import (
	"image"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The limits of the camera. Looking straight down or up would turn the
// camera's own up direction into the direction it looks in, which leaves it
// nothing to tell left from right by.
const (
	maxPitch = 89 * math.Pi / 180

	minDistance = 0.05
	maxDistance = 100

	// fieldOfView is how much the camera sees, top to bottom, in degrees.
	fieldOfView = 30
)

// Viewer draws models into a picture of its own, from a camera that circles
// them: turned around them by its yaw, tilted over them by its pitch, and
// moved closer or further by its distance.
//
// A yaw of zero looks at a model from the front, the side a portrait shows:
// the side the game's own camera looks at, along +z in its coordinates.
type Viewer struct {
	renderer *Renderer
	target   rl.RenderTexture2D

	// Yaw turns the camera around the model and Pitch tilts it over it, both
	// in radians; a positive pitch looks down. Rotate stops the pitch short
	// of straight down or up, but set directly it can be either, for a view
	// from the top or the bottom.
	Yaw, Pitch float64

	// Distance is how far the camera is from the point it looks at, in
	// multiples of the size the framed model fits in.
	Distance float64

	// FieldOfView is how much the camera sees, top to bottom, in degrees;
	// zero for the viewer's own, which shows a framed model whole.
	FieldOfView float64

	// Look is what the models are drawn with.
	Look Look

	// Background is the colour behind the models.
	Background rl.Color

	// shadow is the depth the shadow camera of the look's lighting sees;
	// see shadow.go.
	shadow shadowTarget

	// occlusion is the depth the camera sees and the ambient occlusion
	// worked out from it; see occlusion.go.
	occlusion occlusionTarget

	// centre is the point the camera looks at, and size how large what it
	// frames is, from Frame.
	centre rl.Vector3
	size   float64
}

// NewViewer prepares a picture of the given size. It needs the OpenGL
// context.
func (r *Renderer) NewViewer(width, height int32) *Viewer {
	viewer := &Viewer{renderer: r, Look: DefaultLook, Background: rl.Blank, size: 1}
	viewer.Resize(width, height)
	viewer.Reset()

	return viewer
}

// Resize changes the size of the picture. A size it has already is left
// alone.
func (v *Viewer) Resize(width, height int32) {
	width, height = max(width, 1), max(height, 1)

	if v.target.ID != 0 {
		if v.target.Texture.Width == width && v.target.Texture.Height == height {
			return
		}

		rl.UnloadRenderTexture(v.target)
	}

	v.target = rl.LoadRenderTexture(width, height)
	rl.SetTextureFilter(v.target.Texture, rl.FilterBilinear)
}

// Size is the size of the picture.
func (v *Viewer) Size() (width, height int32) {
	return v.target.Texture.Width, v.target.Texture.Height
}

// Frame points the camera at the middle of a box, such as the one a model
// fits in, and sizes its distance to it.
func (v *Viewer) Frame(low, high [3]float32) {
	v.centre = rl.Vector3{X: (low[0] + high[0]) / 2, Y: (low[1] + high[1]) / 2, Z: (low[2] + high[2]) / 2}

	extent := math.Max(float64(high[0]-low[0]), math.Max(float64(high[1]-low[1]), float64(high[2]-low[2])))
	v.size = math.Max(extent, 1e-3)
}

// Reset turns the camera back to the front, level, at a distance that shows
// all of what it frames.
func (v *Viewer) Reset() {
	v.Yaw, v.Pitch = 0, 0

	// Far enough for the framed size to fill most of the picture's height.
	v.Distance = 0.6 / math.Tan(v.fov()*math.Pi/360)
}

// Rotate turns the camera around the model by a yaw and tilts it by a pitch,
// in radians. The pitch stops short of straight down or up.
func (v *Viewer) Rotate(yaw, pitch float64) {
	v.Yaw = math.Mod(v.Yaw+yaw, 2*math.Pi)
	v.Pitch = math.Max(-maxPitch, math.Min(maxPitch, v.Pitch+pitch))
}

// Zoom moves the camera closer by a factor above one, further by one below.
func (v *Viewer) Zoom(factor float64) {
	if factor <= 0 {
		return
	}

	v.Distance = math.Max(minDistance, math.Min(maxDistance, v.Distance/factor))
}

// Pan moves the point the camera looks at across the picture: right and up
// as the picture shows them, by as many units of the model, and the camera
// with it, so that the model slides across the picture rather than turning.
// Frame looks at the middle of a box again.
func (v *Viewer) Pan(right, up float64) {
	camera := v.Camera()

	forward := rl.Vector3Normalize(rl.Vector3Subtract(camera.Target, camera.Position))
	screenRight := rl.Vector3Normalize(rl.Vector3CrossProduct(forward, camera.Up))
	screenUp := rl.Vector3CrossProduct(screenRight, forward)

	moved := rl.Vector3Add(rl.Vector3Scale(screenRight, float32(right)), rl.Vector3Scale(screenUp, float32(up)))
	v.centre = rl.Vector3Add(v.centre, moved)
}

// UnitsPerPixel is how many units of the model a pixel of the picture spans
// at the depth of the point the camera looks at, which a drag across the
// picture pans the view by for what was under the pointer to stay under it.
func (v *Viewer) UnitsPerPixel() float64 {
	_, height := v.Size()
	visible := 2 * v.Distance * v.size * math.Tan(v.fov()*math.Pi/360)

	return visible / float64(max(height, 1))
}

// fov is the field of view the camera sees with, in degrees.
func (v *Viewer) fov() float64 {
	if v.FieldOfView > 0 {
		return v.FieldOfView
	}

	return fieldOfView
}

// LookFrom stands the camera at a place, looking at a point, seeing as much as
// a field of view, in degrees, as the cameras of the games' portraits are
// written; zero keeps the viewer's own. Turning and zooming go on around the
// point from there, and Frame looks at a box again.
func (v *Viewer) LookFrom(position, target [3]float32, fieldOfView float64) {
	offset := [3]float64{float64(position[0] - target[0]), float64(position[1] - target[1]), float64(position[2] - target[2])}
	distance := math.Sqrt(offset[0]*offset[0] + offset[1]*offset[1] + offset[2]*offset[2])

	if distance == 0 {
		return
	}

	// The distance is kept as a few multiples of a size, as Frame keeps it,
	// so that zooming moves as far from it as from a framed model.
	const multiples = 2

	v.centre = rl.Vector3{X: target[0], Y: target[1], Z: target[2]}
	v.size = distance / multiples
	v.Distance = multiples
	v.Pitch = math.Asin(offset[1] / distance)
	v.Yaw = math.Atan2(offset[0], -offset[2])
	v.FieldOfView = fieldOfView
}

// Camera is the camera the viewer draws with.
func (v *Viewer) Camera() rl.Camera3D {
	distance := v.Distance * v.size

	// From the front, the camera is on the side of -z, where the game's own
	// camera is, and turning by the yaw takes it around the model to its
	// left.
	offset := rl.Vector3{
		X: float32(distance * math.Cos(v.Pitch) * math.Sin(v.Yaw)),
		Y: float32(distance * math.Sin(v.Pitch)),
		Z: float32(-distance * math.Cos(v.Pitch) * math.Cos(v.Yaw)),
	}

	// Up is up as far as the camera can see it: tilted with the camera, so
	// that it stays at right angles to where it looks. Unlike the world's
	// up, that still tells left from right looking straight down or up, as
	// a view from the top or the bottom does; there, the top of the picture
	// is the back of the model, and from the bottom its front.
	up := rl.Vector3{
		X: float32(-math.Sin(v.Pitch) * math.Sin(v.Yaw)),
		Y: float32(math.Cos(v.Pitch)),
		Z: float32(math.Sin(v.Pitch) * math.Cos(v.Yaw)),
	}

	return rl.Camera3D{
		Position:   rl.Vector3Add(v.centre, offset),
		Target:     v.centre,
		Up:         up,
		Fovy:       float32(v.fov()),
		Projection: rl.CameraPerspective,
	}
}

// Draw draws models into the picture. It has to run while raylib drawing is
// active and before anything samples the picture.
func (v *Viewer) Draw(models ...*Model) {
	// The shadows are drawn first, from where they are cast, into a picture
	// of their own.
	var (
		shadow     *Shadow
		lightSpace rl.Matrix
		drawn      bool
	)

	if v.Look.Lighting != nil && v.Look.Lighting.Shadow != nil {
		shadow = v.Look.Lighting.Shadow
		lightSpace, drawn = v.shadow.draw(v.renderer, shadow, models)
	}

	// Then the ambient occlusion, from the depth the camera sees, into
	// pictures of their own as well.
	var (
		occlusion *Occlusion
		occluded  rl.Texture2D
		worked    bool
	)

	camera := v.Camera()
	near, far := v.planes(models)
	width, height := v.Size()

	if v.Look.Lighting != nil && v.Look.Lighting.Occlusion != nil {
		occlusion = v.Look.Lighting.Occlusion
		occluded, worked = v.occlusion.draw(v.renderer, occlusion, models, camera, near, far, width, height)
	}

	rl.BeginTextureMode(v.target)
	defer rl.EndTextureMode()

	rl.ClearBackground(v.Background)
	v.renderer.applyOcclusion(occlusion, occluded, worked, width, height)
	v.drawModels(models, shadow, lightSpace, drawn)
}

// drawModels draws the models from the camera into the current target.
func (v *Viewer) drawModels(models []*Model, shadow *Shadow, lightSpace rl.Matrix, shadowed bool) {
	camera := v.Camera()

	v.renderer.apply(v.Look, camera.Position)
	v.renderer.applyShadow(shadow, &v.shadow, lightSpace, shadowed)

	// Near and far planes to suit the models drawn, rather than raylib's
	// fixed ones, which are too far apart for a small model and too
	// close together for a large one. BeginMode3D builds the projection from
	// the planes in effect, so they are set first. The ambient occlusion was
	// worked out from a depth drawn between the same.
	near, far := v.planes(models)
	rl.SetClipPlanes(near, far)

	rl.BeginMode3D(camera)

	// What is solid of every model is drawn before what is translucent of
	// any, so that a lens lies over the eyes behind it whichever model each
	// is of.
	for _, drawn := range models {
		if drawn != nil {
			drawn.drawSolid(rl.MatrixIdentity())
		}
	}

	for _, drawn := range models {
		if drawn != nil {
			drawn.drawTranslucent(rl.MatrixIdentity())
		}
	}

	rl.EndMode3D()
}

// planes are the near and far clip planes for the models drawn: as close
// together as they can be and still hold all of them, for the depth to tell
// apart what is close, whatever is framed. Framing a face of a whole body
// must not cut off the feet.
//
// The planes hold a ball around what is framed and the boxes the models fit
// in, with room to spare, since a model's box is that of its meshes as they
// rest and its animations move it beyond. A model hung from a joint has its
// box in the joint's space, so it is left to the room the one it hangs from
// leaves.
func (v *Viewer) planes(models []*Model) (near, far float64) {
	low := [3]float64{float64(v.centre.X) - v.size, float64(v.centre.Y) - v.size, float64(v.centre.Z) - v.size}
	high := [3]float64{float64(v.centre.X) + v.size, float64(v.centre.Y) + v.size, float64(v.centre.Z) + v.size}

	for _, drawn := range models {
		if drawn == nil || drawn.hangsFrom != nil || drawn.Min == drawn.Max {
			continue
		}

		for axis := range 3 {
			low[axis] = math.Min(low[axis], float64(drawn.Min[axis]))
			high[axis] = math.Max(high[axis], float64(drawn.Max[axis]))
		}
	}

	var middle [3]float64

	radius := 0.0

	for axis := range 3 {
		middle[axis] = (low[axis] + high[axis]) / 2
		radius += (high[axis] - low[axis]) * (high[axis] - low[axis]) / 4
	}

	radius = math.Sqrt(radius) * planeRoom

	position := v.Camera().Position
	offset := [3]float64{float64(position.X) - middle[0], float64(position.Y) - middle[1], float64(position.Z) - middle[2]}
	distance := math.Sqrt(offset[0]*offset[0] + offset[1]*offset[1] + offset[2]*offset[2])

	// Inside the ball the near plane comes as close as the depth allows to
	// tell apart what is framed.
	near = math.Max(distance-radius, v.Distance*v.size*nearShare)

	return near, distance + radius
}

// planeRoom is how much further than the ball around the models the planes
// reach, for what their animations move beyond the boxes they rest in, and
// nearShare the least the near plane is of the distance to what is framed.
const (
	planeRoom = 1.5
	nearShare = 0.01
)

// Target is the render texture the models are drawn into, which an interface
// shows. OpenGL fills it bottom up, so it is shown flipped.
func (v *Viewer) Target() rl.RenderTexture2D {
	return v.target
}

// Image reads the picture back from the GPU, the right way up.
func (v *Viewer) Image() *image.RGBA {
	captured := rl.LoadImageFromTexture(v.target.Texture)
	defer rl.UnloadImage(captured)

	// OpenGL fills a render target bottom up.
	rl.ImageFlipVertical(captured)

	colors := rl.LoadImageColors(captured)
	defer rl.UnloadImageColors(colors)

	width, height := int(captured.Width), int(captured.Height)
	picture := image.NewRGBA(image.Rect(0, 0, width, height))

	for index, value := range colors[:width*height] {
		picture.SetRGBA(index%width, index/width, value)
	}

	return picture
}

// Unload releases the picture.
func (v *Viewer) Unload() {
	v.shadow.unload()
	v.occlusion.unload()

	if v.target.ID != 0 {
		rl.UnloadRenderTexture(v.target)
		v.target = rl.RenderTexture2D{}
	}
}
