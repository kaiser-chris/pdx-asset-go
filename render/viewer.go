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

	// lit is the picture of half floats the models are drawn into before
	// the post effect, when there is one; see post.go.
	lit rl.RenderTexture2D

	// Yaw turns the camera around the model and Pitch tilts it over it, both
	// in radians; a positive pitch looks down.
	Yaw, Pitch float64

	// Distance is how far the camera is from the point it looks at, in
	// multiples of the size the framed model fits in.
	Distance float64

	// Look is what the models are drawn with.
	Look Look

	// Background is the colour behind the models.
	Background rl.Color

	// Shadows has the models cast the shadows of the sun with the games'
	// shaders.
	Shadows bool

	// centre is the point the camera looks at, and size how large what it
	// frames is, from Frame.
	centre rl.Vector3
	size   float64
}

// NewViewer prepares a picture of the given size. It needs the OpenGL
// context.
func (r *Renderer) NewViewer(width, height int32) *Viewer {
	viewer := &Viewer{renderer: r, Look: DefaultLook, Background: rl.Blank, size: 1, Shadows: true}
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

	v.unloadLit()

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
	v.Distance = 0.6 / math.Tan(fieldOfView*math.Pi/360)
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

	return rl.Camera3D{
		Position:   rl.Vector3Add(v.centre, offset),
		Target:     v.centre,
		Up:         rl.Vector3{Y: 1},
		Fovy:       fieldOfView,
		Projection: rl.CameraPerspective,
	}
}

// Draw draws models into the picture. It has to run while raylib drawing is
// active and before anything samples the picture.
//
// With the games' shaders, the models are drawn into a picture of their
// light first, which the game's post effect turns into the picture shown.
func (v *Viewer) Draw(models ...*Model) {
	v.renderer.shadows.cast = false
	if v.Shadows {
		v.renderer.castShadows(models)
	}

	// The shadows belong to this frame only.
	defer func() { v.renderer.shadows.cast = false }()

	post, err := v.renderer.post()
	if err == nil && post != nil && v.prepareLit() {
		rl.BeginTextureMode(v.lit)
		rl.ClearBackground(rl.Blank)
		v.drawModels(models)
		rl.EndTextureMode()

		rl.BeginTextureMode(v.target)
		rl.ClearBackground(v.Background)
		v.renderer.drawPost(post, v.lit.Texture)
		rl.EndTextureMode()

		return
	}

	rl.BeginTextureMode(v.target)
	defer rl.EndTextureMode()

	rl.ClearBackground(v.Background)
	v.drawModels(models)
}

// PostError says why the games' post effect could not be had, if it could
// not; the models are shown without it then.
func (v *Viewer) PostError() error {
	_, err := v.renderer.post()

	return err
}

// prepareLit makes the picture of light the size of the one shown.
func (v *Viewer) prepareLit() bool {
	width, height := v.Size()
	if v.lit.ID != 0 && v.lit.Texture.Width == width && v.lit.Texture.Height == height {
		return true
	}

	v.unloadLit()

	lit, err := hdrTarget(width, height)
	if err != nil {
		return false
	}

	v.lit = lit

	return true
}

func (v *Viewer) unloadLit() {
	if v.lit.ID != 0 {
		rl.UnloadRenderTexture(v.lit)
		v.lit = rl.RenderTexture2D{}
	}
}

// drawModels draws the models from the camera into the current target.
func (v *Viewer) drawModels(models []*Model) {
	camera := v.Camera()

	v.renderer.apply(v.Look, camera.Position)

	// Near and far planes to suit the size of what is framed, rather than
	// raylib's fixed ones, which are too far apart for a small model and too
	// close together for a large one. BeginMode3D builds the projection from
	// the planes in effect, so they are set first.
	near, far := v.planes()
	rl.SetClipPlanes(near, far)

	rl.BeginMode3D(camera)

	// What the games' effects need to know of the camera, which raylib has
	// just worked out.
	v.renderer.frame = frame{
		view:       rl.GetMatrixModelview(),
		projection: rl.GetMatrixProjection(),
		camera:     camera,
		near:       float32(near),
		far:        float32(far),
	}

	for _, drawn := range models {
		if drawn != nil {
			drawn.Draw(rl.MatrixIdentity())
		}
	}

	rl.EndMode3D()
}

// planes are the near and far clip planes for the current distance.
func (v *Viewer) planes() (near, far float64) {
	distance := v.Distance * v.size

	return math.Max(distance-v.size*2, distance*0.01), distance + v.size*2
}

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
	v.unloadLit()

	if v.target.ID != 0 {
		rl.UnloadRenderTexture(v.target)
		v.target = rl.RenderTexture2D{}
	}
}
