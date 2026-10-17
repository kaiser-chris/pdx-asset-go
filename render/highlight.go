package render

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Highlighting shows where something changes a model: an editor of portraits
// marks the part of a face a gene moves while the gene is pointed at. What
// moves is found by drawing the model both ways, taking a Snapshot of where
// its vertices are each time, and measuring how far each moved between them;
// the vertices that moved are then tinted by how far, with the colour the
// Look highlights in, the furthest the most.
//
// The tint is carried by the alpha of each vertex's colour, which every piece
// is uploaded with at full, so a model that was never highlighted costs
// nothing to draw but four bytes a vertex.

// highlightThreshold is the share of the furthest movement below which a
// vertex is not highlighted at all, so that what a gene moves only a little,
// by the skin's stretching around what it moves, does not blur the edge of
// what it does.
const highlightThreshold = 0.04

// Snapshot is where the vertices of a model are drawn, piece by piece in the
// order the model holds them, nil for a piece that never moves.
type Snapshot [][]float32

// Snapshot is where the vertices of the model are drawn now, as it was last
// morphed and posed, copied, so that it can be morphed and posed again.
func (m *Model) Snapshot() Snapshot {
	var snapshot Snapshot

	for index := range m.parts {
		for _, piece := range m.parts[index].pieces {
			switch {
			case piece.skin != nil:
				snapshot = append(snapshot, clone(piece.skin.movedPositions))
			case piece.morph != nil:
				snapshot = append(snapshot, clone(piece.morph.positions))
			default:
				snapshot = append(snapshot, nil)
			}
		}
	}

	return snapshot
}

// Movement measures how far each vertex moved from one snapshot of a model to
// another, piece by piece, and the furthest any did. A piece that did not
// move in either, or that the two snapshots do not have alike, moved
// nowhere, and is left nil.
func Movement(from, to Snapshot) (distances [][]float32, furthest float32) {
	distances = make([][]float32, max(len(from), len(to)))

	for piece := range min(len(from), len(to)) {
		before, after := from[piece], to[piece]
		if before == nil || after == nil || len(before) != len(after) {
			continue
		}

		moved := make([]float32, len(before)/3)

		for vertex := range moved {
			dx := after[vertex*3] - before[vertex*3]
			dy := after[vertex*3+1] - before[vertex*3+1]
			dz := after[vertex*3+2] - before[vertex*3+2]

			moved[vertex] = float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
			furthest = max(furthest, moved[vertex])
		}

		distances[piece] = moved
	}

	return distances, furthest
}

// Highlight tints the vertices of the model by how far each moved, out of the
// furthest, as Movement measures them: the furthest in the full colour the
// Look highlights in, and those that moved less than a few hundredths of it
// not at all. The furthest is given rather than taken from the distances so
// that several models measured together, such as the head and the body of a
// portrait, are highlighted alike. Nil distances, or a furthest of nothing,
// highlight nothing.
//
// It has to run on the thread that owns the OpenGL context.
func (m *Model) Highlight(distances [][]float32, furthest float32) {
	position := 0

	for index := range m.parts {
		for _, piece := range m.parts[index].pieces {
			var moved []float32
			if position < len(distances) && furthest > 0 {
				moved = distances[position]
			}

			position++

			if piece.highlight(moved, furthest) {
				rl.UpdateMeshBuffer(*piece.mesh, vertexBufferColor, piece.colors, 0)
			}
		}
	}
}

// highlight sets the alpha of each vertex's colour by how far it moved, and
// reports whether any changed. A vertex the distances do not reach is not
// highlighted.
func (p *gpuPiece) highlight(moved []float32, furthest float32) bool {
	changed := false

	for vertex := range len(p.colors) / 4 {
		amount := float32(0)
		if vertex < len(moved) {
			amount = highlightAmount(moved[vertex] / furthest)
		}

		alpha := byte(math.Round(float64(255 * (1 - amount))))
		if p.colors[vertex*4+3] != alpha {
			p.colors[vertex*4+3] = alpha
			changed = true
		}
	}

	return changed
}

// highlightAmount is how strongly a vertex is highlighted that moved a share
// of the furthest: nothing below the threshold, then rising faster than the
// share, so that what a gene moves stands out as a whole rather than only at
// the point it moves the most.
func highlightAmount(share float32) float32 {
	if share <= highlightThreshold {
		return 0
	}

	return float32(math.Sqrt(float64(min((share-highlightThreshold)/(1-highlightThreshold), 1))))
}

// vertexBufferColor is the vertex buffer raylib keeps a mesh's colours in.
const vertexBufferColor = 3
