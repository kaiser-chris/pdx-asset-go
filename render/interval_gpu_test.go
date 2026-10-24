//go:build uitest

package render

import (
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// Each part takes the hair colour as far as its own entity remaps its colour
// mask: hair whose mask says little, remapped from 0.9 on as the games' hair
// is, is coloured almost wholly, where the look's whole interval leaves it
// nearly white; and a mask of nothing at all takes no colour, whatever the
// interval, as the games' GetColorMaskColorBLend has it.
func TestEachPartRemapsItsColourMask(t *testing.T) {
	renderer := withRenderer(t)

	look := DefaultLook
	look.HairColor = [3]float32{0.2, 0.2, 1}

	draw := func(mask byte, interval *[2]float32) (red, blue int) {
		t.Helper()

		built := quadModel(t, meshtest.Quad(2, 2))
		built.Parts[0].Shader = "portrait_hair"
		built.Parts[0].Textures.Diffuse = pixelOf([4]byte{255, 255, 255, 255})
		built.Parts[0].Textures.Normal = pixelOf([4]byte{128, 128, mask, 128})
		built.Parts[0].ColorMaskInterval = interval

		uploaded, err := renderer.Upload(built)
		if err != nil {
			t.Fatal(err)
		}
		defer uploaded.Unload()

		middle := centre(drawWith(t, renderer, uploaded, look))

		return int(middle.R), int(middle.B)
	}

	hair := &[2]float32{0.9, 1}

	if red, blue := draw(40, nil); red*3 < blue*2 {
		t.Errorf("a little mask over the whole interval is %d red to %d blue, want it nearly white", red, blue)
	}

	if red, blue := draw(40, hair); red*2 > blue {
		t.Errorf("a little mask remapped from 0.9 on is %d red to %d blue, want it coloured nearly wholly", red, blue)
	}

	if red, blue := draw(0, hair); red*3 < blue*2 {
		t.Errorf("no mask remapped from 0.9 on is %d red to %d blue, want it uncoloured", red, blue)
	}
}
