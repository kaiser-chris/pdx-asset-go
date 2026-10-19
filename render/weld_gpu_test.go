//go:build uitest

package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// The vertices of a model welded to another are drawn where the other's are,
// wherever its pose takes them, without the model following it by any
// joint; vertices that lie nowhere near the other's are not welded.
func TestWeldDrawsWhereTheOtherIs(t *testing.T) {
	renderer := withRenderer(t)

	other, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Unload()

	welded, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer welded.Unload()

	apart, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(4, 4)))
	if err != nil {
		t.Fatal(err)
	}
	defer apart.Unload()

	if joined := apart.Weld(other, 0.01); joined != 0 {
		t.Errorf("welded %d vertices of a quad twice the size, want none", joined)
	}

	if joined := welded.Weld(other, 0.01); joined != 4 {
		t.Fatalf("welded %d vertices of the same quad, want its 4", joined)
	}

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()
	viewer.Frame(welded.Min, welded.Max)

	draw := func() int {
		rl.BeginDrawing()
		viewer.Draw(welded)
		rl.EndDrawing()

		return covered(viewer.Image())
	}

	welded.Pose(nil, 0, false, nil)

	rest := draw()
	if rest == 0 {
		t.Fatal("nothing drawn of the welded quad at rest")
	}

	// The other's animation takes every vertex a long way along x, and the
	// welded quad, drawn alone and posed after it, goes with it.
	other.Pose(translationAnimation(t), 0.1, false, []int{0})
	welded.Pose(nil, 0, false, nil)

	if count := draw(); count != 0 {
		t.Errorf("%d pixels of the welded quad drawn, want it gone with the other", count)
	}

	// Let go, it is drawn where it rests whatever the other does.
	welded.Weld(nil, 0)
	welded.Pose(nil, 0, false, nil)

	if count := draw(); count != rest {
		t.Errorf("%d pixels of the quad drawn after letting go, want %d", count, rest)
	}
}
