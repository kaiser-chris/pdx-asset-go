package render

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The camera can look straight down or up, as a view from the top or the
// bottom does, and still knows which way is up in its picture.
func TestCameraLooksStraightDownAndUp(t *testing.T) {
	for _, test := range []struct {
		name       string
		yaw, pitch float64
		above      bool
		up         rl.Vector3
	}{
		{"front", 0, 0, false, rl.Vector3{Y: 1}},
		// From the top, the back of the model is at the top of the picture.
		{"top", 0, math.Pi / 2, true, rl.Vector3{Z: 1}},
		// From the bottom, its front is.
		{"bottom", 0, -math.Pi / 2, false, rl.Vector3{Z: -1}},
		// Turned to the left side, the top view turns with it.
		{"top from the left", math.Pi / 2, math.Pi / 2, true, rl.Vector3{X: -1}},
	} {
		viewer := &Viewer{size: 1}
		viewer.Reset()
		viewer.Yaw, viewer.Pitch = test.yaw, test.pitch

		camera := viewer.Camera()

		if rl.Vector3Distance(camera.Up, test.up) > 1e-5 {
			t.Errorf("%s: up = %v, want %v", test.name, camera.Up, test.up)
		}

		forward := rl.Vector3Normalize(rl.Vector3Subtract(camera.Target, camera.Position))
		if dot := rl.Vector3DotProduct(forward, camera.Up); math.Abs(float64(dot)) > 1e-5 {
			t.Errorf("%s: up is not at right angles to where the camera looks: %v", test.name, dot)
		}

		if above := camera.Position.Y > camera.Target.Y+0.5; above != test.above && test.pitch != 0 {
			t.Errorf("%s: camera at %v, above the model %v, want %v", test.name, camera.Position, above, test.above)
		}
	}
}
