//go:build uitest

package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// A model that hangs from a joint of another is drawn wherever the other's
// animation takes the joint, and where its mesh puts it once let go.
func TestHangMovesWithTheJoint(t *testing.T) {
	renderer := withRenderer(t)

	parent, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Unload()

	child, err := renderer.Upload(quadModel(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer child.Unload()

	// One viewer looks at where the child's mesh puts it, the other at
	// where the parent's animation takes bone0: a hundred along x in the
	// files, a hundred the other way drawn.
	home := renderer.NewViewer(64, 64)
	defer home.Unload()
	home.Frame(child.Min, child.Max)

	away := renderer.NewViewer(64, 64)
	defer away.Unload()
	away.Frame([3]float32{child.Min[0] - 100, child.Min[1], child.Min[2]}, [3]float32{child.Max[0] - 100, child.Max[1], child.Max[2]})

	draw := func() (atHome, atAway int) {
		rl.BeginDrawing()
		home.Draw(child)
		away.Draw(child)
		rl.EndDrawing()

		return covered(home.Image()), covered(away.Image())
	}

	rest, _ := draw()
	if rest == 0 {
		t.Fatal("nothing drawn of the child where its mesh puts it")
	}

	// Hung from bone0 at rest, which is at the origin, it stays put.
	child.Hang(parent, "bone0")
	parent.Pose(nil, 0, false, nil)

	if atHome, atAway := draw(); atHome != rest || atAway != 0 || !child.Hanging() {
		t.Errorf("hung from a joint at rest: %d pixels at home and %d away, want %d and none", atHome, atAway, rest)
	}

	// The parent's animation takes the joint, and the child with it.
	parent.Pose(translationAnimation(t), 0.1, false, []int{0})

	if atHome, atAway := draw(); atHome != 0 || atAway != rest {
		t.Errorf("hung from a joint moved away: %d pixels at home and %d away, want none and %d", atHome, atAway, rest)
	}

	// A joint the parent does not have, or letting go, leaves it home.
	child.Hang(parent, "nowhere")

	if atHome, _ := draw(); atHome != rest || child.Hanging() {
		t.Errorf("hung from a joint that is not there: %d pixels at home, want %d", atHome, rest)
	}

	child.Hang(nil, "")

	if atHome, _ := draw(); atHome != rest {
		t.Errorf("let go: %d pixels at home, want %d", atHome, rest)
	}
}

// A transform is handed to raylib as the same placement: the point it places
// a row at is where raylib's matrix places the column.
func TestMatrixOf(t *testing.T) {
	transform := mat.Turning(1, 0.7).Then(mat.Scale([3]float64{2, 3, 4}))
	transform.Offset = [3]float64{5, -6, 7}

	point := [3]float64{1, 2, 3}
	want := transform.Point(point)

	got := rl.Vector3Transform(rl.Vector3{X: 1, Y: 2, Z: 3}, matrixOf(transform))

	for axis, value := range []float32{got.X, got.Y, got.Z} {
		if diff := float64(value) - want[axis]; diff > 1e-4 || diff < -1e-4 {
			t.Errorf("raylib places %v at %v, the transform at %v", point, got, want)

			break
		}
	}
}
