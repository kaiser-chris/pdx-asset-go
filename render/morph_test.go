package render

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/model"
)

// Blending adds each shape's offsets times its weight, and normalises the
// directions again.
func TestBlend(t *testing.T) {
	piece := &model.Piece{
		Positions: []float32{1, 0, 0},
		Normals:   []float32{0, 0, 1},
		Tangents:  []float32{1, 0, 0, -1},
		BlendShapes: []model.BlendShape{
			{ID: "up", Positions: []float32{0, 2, 0}, Normals: []float32{0, 1, -1}, Tangents: []float32{-1, 1, 0}},
			{ID: "out", Positions: []float32{4, 0, 0}, Normals: []float32{0, 0, 0}, Tangents: []float32{0, 0, 0}},
			{ID: "broken", Positions: []float32{1}},
		},
	}

	morph := newMorph(piece)

	morph.weights = []float32{0.5, -0.25, 1}
	morph.blend()

	if morph.positions[0] != 0 || morph.positions[1] != 1 || morph.positions[2] != 0 {
		t.Errorf("position = %v, want 0 1 0", morph.positions)
	}

	// Halfway between facing z and facing y, of unit length.
	half := float32(1 / math.Sqrt2)
	if !close(morph.normals, []float32{0, half, half}) {
		t.Errorf("normal = %v", morph.normals)
	}

	// The tangent turns too, and which way round the bitangent goes stays.
	if !close(morph.tangents, []float32{half, half, 0, -1}) {
		t.Errorf("tangent = %v", morph.tangents)
	}

	// Back to nothing, the piece is as it was converted, which blending
	// never changes.
	morph.weights = []float32{0, 0, 0}
	morph.blend()

	if !close(morph.positions, piece.Positions) || !close(morph.normals, piece.Normals) || piece.Positions[0] != 1 {
		t.Errorf("unmorphed = %v, %v; piece %v", morph.positions, morph.normals, piece.Positions)
	}
}

func close(got, want []float32) bool {
	if len(got) != len(want) {
		return false
	}

	for index := range got {
		if math.Abs(float64(got[index]-want[index])) > 1e-6 {
			return false
		}
	}

	return true
}
