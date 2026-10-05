//go:build uitest

package render

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// withRenderer opens a hidden window for a test that needs an OpenGL context,
// and a renderer in it. The context belongs to the thread that created it,
// and a test runs on a goroutine of its own, so each test opens one and stays
// on its thread.
func withRenderer(t *testing.T) *Renderer {
	t.Helper()

	runtime.LockOSThread()

	rl.SetTraceLogLevel(rl.LogWarning)
	rl.SetConfigFlags(rl.FlagWindowHidden)
	rl.InitWindow(64, 64, "render tests")

	renderer, err := NewRenderer()
	if err != nil {
		rl.CloseWindow()
		runtime.UnlockOSThread()
		t.Fatalf("NewRenderer: %v", err)
	}

	t.Cleanup(func() {
		renderer.Unload()
		rl.CloseWindow()
		runtime.UnlockOSThread()
	})

	return renderer
}

// quadModel is a square facing the game's camera, its texture red on its left
// half and blue on its right, as the game would show it.
func quadModel(t *testing.T, geometry meshtest.Geometry) *model.Model {
	t.Helper()

	file, err := mesh.Read(meshtest.New().Object(1, "object").Object(2, "s").Mesh(geometry, "").Bytes())
	if err != nil {
		t.Fatal(err)
	}

	halves := &texture.Image{Width: 2, Height: 1, Format: texture.RGBA8, Levels: 1, Data: []byte{
		255, 0, 0, 255,
		0, 0, 255, 255,
	}}

	built := &model.Model{Name: "quad", Parts: []model.Part{{
		Name:     "quad",
		Pieces:   model.Convert(&file.Shapes[0].Meshes[0]),
		Textures: model.Textures{Diffuse: halves},
	}}}
	built.Bounds()

	return built
}

// render uploads a model and draws it from a camera turned by a yaw.
func render(t *testing.T, renderer *Renderer, source *model.Model, yaw float64) *image.RGBA {
	t.Helper()

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Rotate(yaw, 0)

	// The colour mask is left out, so the texture's colours come through as
	// they are.
	viewer.Look.PaletteColor = [3]float32{1, 1, 1}

	rl.BeginDrawing()
	viewer.Draw(uploaded)
	rl.EndDrawing()

	return viewer.Image()
}

// covered counts the pixels something was drawn on.
func covered(picture *image.RGBA) int {
	count := 0

	for index := 3; index < len(picture.Pix); index += 4 {
		if picture.Pix[index] > 0 {
			count++
		}
	}

	return count
}

// dominant says which colour channel of a pixel is strongest.
func dominant(pixel color.RGBA) string {
	switch {
	case pixel.A == 0:
		return "nothing"
	case pixel.R > pixel.G && pixel.R > pixel.B:
		return "red"
	case pixel.B > pixel.R && pixel.B > pixel.G:
		return "blue"
	}

	return "grey"
}

// Seen from the front, the model is the way round the game shows it: the left
// of its texture on the left of the picture. Mirroring it into OpenGL's
// coordinates without winding its triangles the other way round would draw
// nothing at all, since every triangle would then face away.
func TestDrawsTheFrontTheWayTheGameDoes(t *testing.T) {
	renderer := withRenderer(t)
	picture := render(t, renderer, quadModel(t, meshtest.Quad(2, 2)), 0)

	if left, right := dominant(picture.RGBAAt(20, 32)), dominant(picture.RGBAAt(44, 32)); left != "red" || right != "blue" {
		t.Errorf("left of the picture is %s, right %s; want red then blue, as the game shows the quad", left, right)
	}

	// The square fills most of the picture, and leaves its corners.
	if count := covered(picture); count < 64*64/4 || count >= 64*64 {
		t.Errorf("%d of %d pixels drawn", count, 64*64)
	}

	if corner := picture.RGBAAt(0, 0); corner.A != 0 {
		t.Errorf("corner = %v, want the background", corner)
	}
}

// From behind, the quad shows its back, which is not drawn.
func TestBackFacesAreNotDrawn(t *testing.T) {
	renderer := withRenderer(t)

	if count := covered(render(t, renderer, quadModel(t, meshtest.Quad(2, 2)), math.Pi)); count != 0 {
		t.Errorf("%d pixels drawn of the back of a quad", count)
	}
}

// Turning the camera around the model changes what is seen: side on, the
// flat quad all but disappears.
func TestTurningTheCamera(t *testing.T) {
	renderer := withRenderer(t)
	quad := quadModel(t, meshtest.Quad(2, 2))

	front := covered(render(t, renderer, quad, 0))
	side := covered(render(t, renderer, quad, math.Pi/2-0.05))

	if side >= front/4 {
		t.Errorf("%d pixels drawn side on, %d from the front; want far fewer side on", side, front)
	}
}

// A mesh of more vertices than sixteen bit indices reach is drawn whole, from
// the pieces it was cut into.
func TestDrawsLargeMeshesWhole(t *testing.T) {
	renderer := withRenderer(t)

	large := quadModel(t, meshtest.Grid(400, 300))
	if pieces := len(large.Parts[0].Pieces); pieces < 2 {
		t.Fatalf("%d pieces, want the grid cut up", pieces)
	}

	small := quadModel(t, meshtest.Grid(4, 3))

	// The same rectangle, finely or coarsely divided, covers the same pixels.
	if fine, coarse := covered(render(t, renderer, large, 0)), covered(render(t, renderer, small, 0)); fine == 0 || fine != coarse {
		t.Errorf("the finely divided grid covers %d pixels, the coarse one %d", fine, coarse)
	}
}

// Compressed textures reach the GPU as they are and are drawn in their
// colours, which shows the pixel formats raylib is told are the right ones.
func TestDrawsCompressedTextures(t *testing.T) {
	renderer := withRenderer(t)
	quad := quadModel(t, meshtest.Quad(2, 2))

	// One DXT5 block, all of it opaque red: an alpha block of both ends 255,
	// and a colour block of both ends pure red in five six five.
	block := []byte{255, 255, 0, 0, 0, 0, 0, 0, 0x00, 0xf8, 0x00, 0xf8, 0, 0, 0, 0}
	quad.Parts[0].Textures.Diffuse = &texture.Image{Width: 4, Height: 4, Format: texture.DXT5, Levels: 1, Data: block}

	picture := render(t, renderer, quad, 0)

	if left, right := dominant(picture.RGBAAt(20, 32)), dominant(picture.RGBAAt(44, 32)); left != "red" || right != "red" {
		t.Errorf("the quad is %s and %s, want red all over", left, right)
	}
}

// A texture several parts share is uploaded once, and unloading a model
// leaves the renderer able to draw the next one.
func TestModelsShareTexturesAndUnloadCleanly(t *testing.T) {
	renderer := withRenderer(t)

	quad := quadModel(t, meshtest.Quad(2, 2))
	quad.Parts = append(quad.Parts, quad.Parts[0])

	uploaded, err := renderer.Upload(quad)
	if err != nil {
		t.Fatal(err)
	}

	if len(uploaded.textures) != 1 || len(uploaded.parts) != 2 {
		t.Errorf("%d textures for %d parts, want the shared one uploaded once", len(uploaded.textures), len(uploaded.parts))
	}

	uploaded.Unload()
	uploaded.Unload()

	// The stand in textures and the shader still work after a model that
	// used them is gone.
	plain := quadModel(t, meshtest.Quad(2, 2))
	plain.Parts[0].Textures = model.Textures{}

	if count := covered(render(t, renderer, plain, 0)); count == 0 {
		t.Error("nothing drawn after a model was unloaded")
	}
}

func TestUploadRefusesWhatCannotBeDrawn(t *testing.T) {
	renderer := withRenderer(t)

	for name, broken := range map[string]*model.Model{
		"an empty piece":      {Parts: []model.Part{{Pieces: []model.Piece{{}}}}},
		"an unknown format":   {Parts: []model.Part{{Textures: model.Textures{Diffuse: &texture.Image{Width: 1, Height: 1, Levels: 1, Data: []byte{1, 2, 3, 4}}}}}},
		"a texture too large": {Parts: []model.Part{{Textures: model.Textures{Diffuse: &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: make([]byte, 100)}}}}},
	} {
		if uploaded, err := renderer.Upload(broken); err == nil {
			uploaded.Unload()
			t.Errorf("%s was uploaded", name)
		}
	}
}

// The viewer's camera keeps its pitch short of straight up and down, and its
// distance within limits, however far it is turned.
func TestViewerLimits(t *testing.T) {
	renderer := withRenderer(t)

	viewer := renderer.NewViewer(10, 10)
	defer viewer.Unload()

	viewer.Rotate(0, 10)
	if viewer.Pitch > maxPitch {
		t.Errorf("pitch = %v, want at most %v", viewer.Pitch, maxPitch)
	}

	viewer.Rotate(0, -20)
	if viewer.Pitch < -maxPitch {
		t.Errorf("pitch = %v, want at least %v", viewer.Pitch, -maxPitch)
	}

	viewer.Zoom(1e9)
	if viewer.Distance < minDistance {
		t.Errorf("distance = %v, want at least %v", viewer.Distance, minDistance)
	}

	viewer.Zoom(1e-9)
	if viewer.Distance > maxDistance {
		t.Errorf("distance = %v, want at most %v", viewer.Distance, maxDistance)
	}

	viewer.Zoom(0)
	viewer.Zoom(-1)

	viewer.Reset()
	if viewer.Yaw != 0 || viewer.Pitch != 0 {
		t.Errorf("after Reset yaw %v and pitch %v", viewer.Yaw, viewer.Pitch)
	}

	viewer.Resize(20, 30)
	if width, height := viewer.Size(); width != 20 || height != 30 {
		t.Errorf("size = %dx%d after resizing to 20x30", width, height)
	}

	viewer.Resize(0, -5)
	if width, height := viewer.Size(); width != 1 || height != 1 {
		t.Errorf("size = %dx%d after resizing to nothing, want 1x1", width, height)
	}
}
