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

// Panning slides the point the camera looks at along the picture's right and
// up, whichever way the camera is turned, and keeps the camera's distance.
func TestPan(t *testing.T) {
	for _, test := range []struct {
		name      string
		yaw       float64
		right, up rl.Vector3
	}{
		{"front", 0, rl.Vector3{X: -1}, rl.Vector3{Y: 1}},
		{"from the left", math.Pi / 2, rl.Vector3{Z: -1}, rl.Vector3{Y: 1}},
	} {
		viewer := &Viewer{size: 1, Distance: 2, Yaw: test.yaw}
		before := viewer.Camera()

		viewer.Pan(1, 2)
		after := viewer.Camera()

		want := rl.Vector3Add(test.right, rl.Vector3Scale(test.up, 2))
		moved := rl.Vector3Subtract(after.Target, before.Target)

		if rl.Vector3Distance(moved, want) > 1e-4 {
			t.Errorf("%s: the target moved by %v, want %v", test.name, moved, want)
		}

		if got := rl.Vector3Distance(after.Position, after.Target); math.Abs(float64(got)-2) > 1e-4 {
			t.Errorf("%s: the camera is %v from what it looks at, want 2", test.name, got)
		}
	}
}

// A camera stood at a place, looking at a point, is where it was stood and
// looks where it was told, with the field of view it was given.
func TestLookFrom(t *testing.T) {
	viewer := &Viewer{size: 1}

	for _, place := range [][3]float32{{0, 3, -150}, {100, 50, -20}, {-30, -10, 40}} {
		target := [3]float32{1, 160, 2}
		position := [3]float32{target[0] + place[0], target[1] + place[1], target[2] + place[2]}

		viewer.LookFrom(position, target, 20)
		camera := viewer.Camera()

		if rl.Vector3Distance(camera.Position, rl.Vector3{X: position[0], Y: position[1], Z: position[2]}) > 1e-3 {
			t.Errorf("stood at %v, want %v", camera.Position, position)
		}

		if camera.Target != (rl.Vector3{X: target[0], Y: target[1], Z: target[2]}) || camera.Fovy != 20 {
			t.Errorf("looks at %v with %v, want %v with 20", camera.Target, camera.Fovy, target)
		}
	}
}
