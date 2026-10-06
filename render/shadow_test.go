package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/shader"
)

// The camera of the sun sees all of what it casts the shadows of: every
// corner of the box lands inside the shadow map, at a depth from 0 to 1,
// and the corner nearest the light is the least deep.
func TestShadowCamera(t *testing.T) {
	low, high := rl.Vector3{X: -1, Y: 0, Z: -2}, rl.Vector3{X: 3, Y: 5, Z: 1}

	// Victoria 3's: from the side and in front, low.
	toLight := rl.Vector3Normalize(rl.Vector3{X: -0.74, Y: 0.38, Z: -0.55})

	renderer := &Renderer{}
	sun := shadowCamera(low, high, toLight)
	renderer.shadows.view, renderer.shadows.projection = sun.view, sun.projection
	toTexture := renderer.shadowTextureMatrix()

	nearest, nearestDepth, deepest := rl.Vector3{}, float32(2), float32(-1)

	for corner := range 8 {
		point := rl.Vector3{X: low.X, Y: low.Y, Z: low.Z}
		if corner&1 != 0 {
			point.X = high.X
		}
		if corner&2 != 0 {
			point.Y = high.Y
		}
		if corner&4 != 0 {
			point.Z = high.Z
		}

		in := rl.Vector3Transform(point, toTexture)
		for axis, value := range []float32{in.X, in.Y, in.Z} {
			if value < 0 || value > 1 {
				t.Errorf("corner %v is at %v in the shadow map; axis %d outside 0 to 1", point, in, axis)
			}
		}

		if in.Z < nearestDepth {
			nearest, nearestDepth = point, in.Z
		}

		deepest = max(deepest, in.Z)
	}

	// The corner the light reaches first: low x, high y, low z.
	if want := (rl.Vector3{X: low.X, Y: high.Y, Z: low.Z}); nearest != want {
		t.Errorf("least deep corner = %v, want %v", nearest, want)
	}

	if deepest-nearestDepth < 0.1 {
		t.Errorf("depths of the corners span %v to %v; the box is squeezed into too little of the map", nearestDepth, deepest)
	}

	// Straight down, the camera needs an up of its own.
	down := shadowCamera(low, high, rl.Vector3{Y: 1})
	if in := rl.Vector3Transform(rl.Vector3{X: 1, Y: 5, Z: -0.5}, down.view); in.X != in.X {
		t.Error("the camera straight above has no orientation")
	}
}

// The shadows fall along the direction the game gives without the mirror
// the sun is lit by, as Victoria 3's model editor shows them.
func TestShadowLight(t *testing.T) {
	if got := shadowLight([3]float32{-0.74, 0.38, -0.55}); got != (rl.Vector3{X: -0.74, Y: 0.38, Z: -0.55}) {
		t.Errorf("light = %v, want the game's direction as it is", got)
	}
}

// The depth bias of a rasterizer state, as the games' shadow effects set it,
// with the names written in either case.
func TestRasterizerBias(t *testing.T) {
	for _, test := range []struct {
		values       map[string]string
		slope, steps float32
	}{
		// Victoria 3's ShadowRasterizerState.
		{map[string]string{"DepthBias": "0", "SlopeScaleDepthBias": "2"}, 2, 0},
		// Crusader Kings 3's.
		{map[string]string{"DepthBias": "40000", "SlopeScaleDepthBias": "2"}, 2, 40000},
		{map[string]string{"depthbias": "-500", "slopescaledepthbias": "-7", "CullMode": "none"}, -7, -500},
		{map[string]string{"CullMode": "none"}, 0, 0},
	} {
		slope, steps := rasterizerBias(&shader.State{Values: test.values})
		if slope != test.slope || steps != test.steps {
			t.Errorf("%v: bias = %v, %v; want %v, %v", test.values, slope, steps, test.slope, test.steps)
		}
	}

	if slope, steps := rasterizerBias(nil); slope != 0 || steps != 0 {
		t.Error("no state has a bias")
	}
}

// The places around a point the effects compare lie within about one of
// it, which KernelScale scales to pixels of the map, and are all different.
func TestDiscSamples(t *testing.T) {
	seen := map[rl.Vector2]bool{}

	for _, pair := range discSamples {
		for _, sample := range []rl.Vector2{{X: pair[0], Y: pair[1]}, {X: pair[2], Y: pair[3]}} {
			if length := rl.Vector2Length(sample); length > 1.4 {
				t.Errorf("sample %v lies %v from the point", sample, length)
			}

			if seen[sample] {
				t.Errorf("sample %v comes twice", sample)
			}

			seen[sample] = true
		}
	}
}
