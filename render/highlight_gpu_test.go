//go:build uitest

package render

import (
	"image"
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// drawWith draws an uploaded model from the front with a look.
func drawWith(t *testing.T, renderer *Renderer, uploaded *Model, look Look) *image.RGBA {
	t.Helper()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Look = look

	rl.BeginDrawing()
	viewer.Draw(uploaded)
	rl.EndDrawing()

	return viewer.Image()
}

// What a gene moves is found by snapshotting a model before and after, and is
// drawn in the highlight's colour once highlighted, until the highlight is
// cleared.
func TestHighlightTintsWhatMoved(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(narrowed(t, quadModel(t, meshtest.Quad(2, 2))))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	before := uploaded.Snapshot()
	uploaded.Morph(map[string]float64{"narrow": 1})
	after := uploaded.Snapshot()
	uploaded.Morph(nil)

	distances, furthest := Movement(before, after)
	if math.Abs(float64(furthest)-0.5) > 1e-4 {
		t.Fatalf("furthest = %v, want the half a unit each side moves in", furthest)
	}

	look := DefaultLook
	look.PaletteColor = [3]float32{1, 1, 1}
	look.Highlight = [4]float32{0, 0, 1, 1}

	plain := centre(drawWith(t, renderer, uploaded, look))

	uploaded.Highlight(distances, furthest)

	if got := centre(drawWith(t, renderer, uploaded, look)); dominant(got) != "blue" || got.B < 240 {
		t.Errorf("highlighted = %v, want the highlight's blue", got)
	}

	uploaded.Highlight(nil, 0)

	if got := centre(drawWith(t, renderer, uploaded, look)); got != plain {
		t.Errorf("cleared = %v, want %v as before", got, plain)
	}
}

// Eyes take the eye colour and hair the hair colour, each where its own mask
// says: an eye where its diffuse map's alpha does, as skin, and hair where the
// blue of its normal map does.
func TestLookColoursEyesAndHair(t *testing.T) {
	renderer := withRenderer(t)

	look := DefaultLook
	look.PaletteColor = [3]float32{0.2, 1, 0.2}
	look.EyeColor = [3]float32{0.2, 0.2, 1}
	look.HairColor = [3]float32{1, 0.2, 0.2}

	white := halvesOf([4]byte{255, 255, 255, 255}, [4]byte{255, 255, 255, 255})

	draw := func(source *model.Model) [4]uint8 {
		uploaded, err := renderer.Upload(source)
		if err != nil {
			t.Fatal(err)
		}
		defer uploaded.Unload()

		got := centre(drawWith(t, renderer, uploaded, look))

		return [4]uint8{got.R, got.G, got.B, got.A}
	}

	eye := lookQuad(t, "portrait_eye", white)
	if got := draw(eye); got[2] <= got[0] || got[2] <= got[1] {
		t.Errorf("eye = %v, want the eye colour's blue", got)
	}

	// A flat normal of full blue: the mask says hair everywhere.
	masked := lookQuad(t, "portrait_hair", white)
	masked.Parts[0].Textures.Normal = &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: []byte{0, 128, 255, 128}}

	if got := draw(masked); got[0] <= got[1] || got[0] <= got[2] {
		t.Errorf("hair = %v, want the hair colour's red", got)
	}

	// No blue in the normal map: no hair colour.
	bare := lookQuad(t, "portrait_hair", white)
	bare.Parts[0].Textures.Normal = &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: []byte{0, 128, 0, 128}}

	if got := draw(bare); int(got[0]) > int(got[1])+20 {
		t.Errorf("unmasked hair = %v, want no hair colour", got)
	}
}
