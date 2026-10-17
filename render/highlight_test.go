package render

import (
	"math"
	"testing"
)

// Movement measures how far each vertex moved between two snapshots, and
// leaves out the pieces that did not move or that the snapshots do not have
// alike.
func TestMovement(t *testing.T) {
	from := Snapshot{{0, 0, 0, 1, 1, 1}, nil, {0, 0, 0}}
	to := Snapshot{{3, 4, 0, 1, 1, 1}, {5, 5, 5}, {0, 0}}

	distances, furthest := Movement(from, to)

	if furthest != 5 {
		t.Errorf("furthest = %v, want 5", furthest)
	}

	if len(distances) != 3 || len(distances[0]) != 2 || distances[0][0] != 5 || distances[0][1] != 0 {
		t.Errorf("distances = %v, want [5 0] for the first piece", distances)
	}

	if distances[1] != nil || distances[2] != nil {
		t.Errorf("distances = %v, want nothing for a piece that never moves or does not match", distances)
	}
}

// A vertex is highlighted not at all below the threshold, fully at the
// furthest, and more than its share in between.
func TestHighlightAmount(t *testing.T) {
	if got := highlightAmount(highlightThreshold / 2); got != 0 {
		t.Errorf("below the threshold = %v, want 0", got)
	}

	if got := highlightAmount(1); math.Abs(float64(got)-1) > 1e-6 {
		t.Errorf("at the furthest = %v, want 1", got)
	}

	if got := highlightAmount(0.5); got <= 0.5 || got >= 1 {
		t.Errorf("halfway = %v, want between a half and one", got)
	}
}

// A piece's colours are only changed, and reported changed, where the
// highlight changes them.
func TestPieceHighlight(t *testing.T) {
	piece := &gpuPiece{colors: []byte{255, 255, 255, 255, 255, 255, 255, 255}}

	if !piece.highlight([]float32{2, 0}, 2) {
		t.Fatal("highlighting a vertex reported no change")
	}

	if piece.colors[3] != 0 || piece.colors[7] != 255 {
		t.Errorf("alphas = %d and %d, want 0 for the furthest and 255 for the still", piece.colors[3], piece.colors[7])
	}

	if piece.highlight([]float32{2, 0}, 2) {
		t.Error("highlighting the same again reported a change")
	}

	if !piece.highlight(nil, 2) || piece.colors[3] != 255 {
		t.Errorf("clearing left alpha %d, want 255", piece.colors[3])
	}
}
