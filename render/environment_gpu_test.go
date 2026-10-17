//go:build uitest

package render

import (
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// cubeOf is a cubemap of one colour.
func cubeOf(colour [4]byte) [6]*texture.Image {
	var faces [6]*texture.Image

	for index := range faces {
		faces[index] = &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: colour[:]}
	}

	return faces
}

// A Lighting lights a model in the place of the renderer's own lights: by its
// lights, each of its own colour where it reaches, and by its environment.
func TestLightingLightsTheModel(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(quadModel(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	look := DefaultLook
	look.PaletteColor = [3]float32{1, 1, 1}

	// A red light in front of the quad, where the camera is, shining at it.
	look.Lighting = &Lighting{Lights: []Light{{
		Kind:      LightSpot,
		Position:  [3]float32{0, 0, -5},
		Direction: [3]float32{0, 0, 1},
		Color:     [3]float32{8, 0, 0},
		Radius:    100,
		Falloff:   10,
		OuterCone: 0.8,
	}}}

	lit := centre(drawWith(t, renderer, uploaded, look))
	if dominant(lit) != "red" || lit.R < 100 {
		t.Errorf("lit by a red light = %v, want red", lit)
	}

	// A light that does not reach the quad leaves it dark.
	look.Lighting.Lights[0].Radius = 2

	if dark := centre(drawWith(t, renderer, uploaded, look)); dark.R > 10 {
		t.Errorf("lit by a light that does not reach = %v, want dark", dark)
	}

	// A green environment lights it green.
	environment, err := renderer.UploadEnvironment(cubeOf([4]byte{0, 255, 0, 255}))
	if err != nil {
		t.Fatal(err)
	}
	defer environment.Unload()

	look.Lighting = &Lighting{Environment: environment, EnvironmentIntensity: 2}

	if green := centre(drawWith(t, renderer, uploaded, look)); dominant(green) != "green" {
		t.Errorf("lit by a green environment = %v, want green", green)
	}
}
