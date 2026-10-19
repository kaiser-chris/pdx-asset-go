//go:build uitest

package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// moved is a geometry with its vertices moved, in the coordinates of the
// games' files.
func moved(geometry meshtest.Geometry, by [3]float32) meshtest.Geometry {
	for index := range geometry.Positions {
		geometry.Positions[index] += by[index%3]
	}

	return geometry
}

// A part casts its shadow unless its effect is one the games give no shadow
// effect, such as portrait_hair_alpha, which Europa Universalis 5 draws its
// eyelashes with: their shadow would streak the cheeks.
func TestShadowlessEffectsCastNoShadow(t *testing.T) {
	renderer := withRenderer(t)

	wall, err := renderer.Upload(matteModel(t, meshtest.Quad(4, 4), "portrait_skin", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer wall.Unload()

	// The light shines from in front and to the side, so that a quad in
	// front of the wall, to that side, throws its shadow on the middle of
	// the wall, which the camera sees past it. The file's x is mirrored, so
	// the quad at 3 is drawn at -3. The shadow is looked up half a unit off
	// the wall, the size of a portrait's centimetres, which the quad is
	// large and far enough for.
	direction := [3]float32{3, 0, 2}

	look := DefaultLook
	look.PaletteColor = [3]float32{1, 1, 1}
	look.Lighting = &Lighting{
		Lights: []Light{{Kind: LightDirectional, Direction: direction, Color: [3]float32{3, 3, 3}, AffectedByShadow: true}},
		Shadow: &Shadow{
			Position: [3]float32{-7.5, 0, -5}, FieldOfView: 60, Near: 0.5, Far: 30,
			KernelScale: 1, FadeFactor: 1,
		},
		Exposure: 1,
	}

	middle := func(models ...*Model) uint8 {
		viewer := renderer.NewViewer(64, 64)
		defer viewer.Unload()

		viewer.Frame(wall.Min, wall.Max)
		viewer.Look = look

		rl.BeginDrawing()
		viewer.Draw(models...)
		rl.EndDrawing()

		picture := viewer.Image()

		return picture.RGBAAt(picture.Bounds().Dx()/2, picture.Bounds().Dy()/2).R
	}

	lit := middle(wall)
	if lit < 40 {
		t.Fatalf("the middle of the wall is %d lit, want it lit to see a shadow on it", lit)
	}

	for _, test := range []struct {
		shader string
		shadow bool
	}{{"portrait_attachment", true}, {"portrait_hair_alpha", false}, {"portrait_eye", false}} {
		built := matteModel(t, moved(meshtest.Quad(2, 2), [3]float32{3, 0, -2}), test.shader, nil)
		built.Parts[0].Textures.Diffuse = pixelOf([4]byte{255, 255, 255, 255})
		built.Bounds()

		occluder, err := renderer.Upload(built)
		if err != nil {
			t.Fatal(err)
		}

		got := middle(wall, occluder)
		occluder.Unload()

		if shadowed := int(got) < int(lit)-20; shadowed != test.shadow {
			t.Errorf("%s: the middle of the wall is %d, %d without it; want shadowed %v", test.shader, got, lit, test.shadow)
		}
	}
}
