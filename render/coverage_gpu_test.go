//go:build uitest

package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// A part drawn with alpha to coverage, such as the lenses of glasses, is
// solid where its alpha is whole and lets what is behind it show through
// elsewhere, even what is of a model drawn after it.
func TestCoverageLetsWhatIsBehindShowThrough(t *testing.T) {
	renderer := withRenderer(t)

	// Blue, solid on the left and half covering on the right.
	lens := lookQuad(t, "portrait_attachment_alpha_to_coverage", halvesOf([4]byte{0, 0, 255, 255}, [4]byte{0, 0, 255, 128}))

	behind := quadModel(t, pushedBack(meshtest.Quad(4, 4), 1))
	behind.Parts[0].Textures.Diffuse = pixelOf([4]byte{255, 0, 0, 255})

	front, err := renderer.Upload(lens)
	if err != nil {
		t.Fatal(err)
	}
	defer front.Unload()

	back, err := renderer.Upload(behind)
	if err != nil {
		t.Fatal(err)
	}
	defer back.Unload()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(front.Min, front.Max)
	viewer.Look.PaletteColor = [3]float32{1, 1, 1}

	rl.BeginDrawing()
	viewer.Draw(front, back)
	rl.EndDrawing()

	picture := viewer.Image()
	middle := picture.Bounds().Dy() / 2

	// The left of the texture is on the left of the picture, as
	// TestDrawsTheFrontTheWayTheGameDoes shows.
	solid, through := picture.RGBAAt(16, middle), picture.RGBAAt(48, middle)

	if dominant(solid) != "blue" {
		t.Errorf("solid half %v, want blue", solid)
	}

	if int(through.R) < int(solid.R)+50 || through.B < 60 {
		t.Errorf("half covering half %v, want the red behind it showing through the blue", through)
	}
}

// pushedBack moves a geometry away from the games' camera.
func pushedBack(geometry meshtest.Geometry, by float32) meshtest.Geometry {
	for index := 2; index < len(geometry.Positions); index += 3 {
		geometry.Positions[index] += by
	}

	return geometry
}
