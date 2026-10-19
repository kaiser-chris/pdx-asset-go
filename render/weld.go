package render

import (
	"math"
	"slices"
	"unsafe"
)

// A portrait's head and body are two meshes that meet in a ring of vertices
// at the same places, where the neck of the one ends and the collar of the
// other begins. The two do not move the ring alike: the head's blend shapes
// widen and shape the neck, and its own bones carry it, while nothing of the
// body follows, so the ring opens into a crack the background shows through,
// and the light falls on either side of it differently. The games dress
// their portraits over it, collar and all; a portrait drawn bare shows it.
//
// Weld closes it: the vertices of the model's ring are drawn wherever the
// other's are, as they are posed and morphed now, with their normals, so the
// two surfaces meet without a crack or a crease.
//
// The rings do not always lie at the same places as the meshes rest: in
// Europa Universalis 5 a woman's head ends a tenth of a unit from her body
// in places, and a boy is drawn with a man's head, whose ring a blend shape
// the age sets moves onto the narrower body of a boy. So the ring is found by
// the open edges of the two meshes, where each ends, and each vertex of the
// one taken to the nearest of the other within a tolerance wide enough for
// the two to be apart, but narrower than the space between the vertices of
// a ring.

// weld is a vertex of a piece of the model drawn where a vertex of a piece
// of the model it is welded to is.
type weld struct {
	vertex int
	from   *gpuPiece
	at     int
}

// Weld joins the vertices on the open edges of the model, where its mesh
// ends, to the nearest vertex on an open edge of another within a tolerance,
// as the models rest, and reports how many it joined. Only the parts of each
// model's own entity are joined, not those that share its pose, such as
// hair, whose ends may hang near the other's edge. From then on each is drawn where the other's is
// drawn, wherever the other's pose and blend shapes take it, with its normal
// and its tangent. A nil other lets go again.
//
// Only skinned pieces are joined, whose vertices are moved every time the
// model is posed. A model welded to another has to be posed after it, as
// one that follows it is; Pose always poses it.
func (m *Model) Weld(other *Model, tolerance float64) int {
	// The vertices are posed again either way: welded, or back where their
	// own bones take them.
	m.welded, m.welds, m.morphed = other, nil, true

	if other == nil {
		return 0
	}

	cell := math.Max(tolerance, 1e-6)
	key := func(positions []float32, at int) [3]int64 {
		return [3]int64{
			int64(math.Floor(float64(positions[at*3]) / cell)),
			int64(math.Floor(float64(positions[at*3+1]) / cell)),
			int64(math.Floor(float64(positions[at*3+2]) / cell)),
		}
	}

	// The other's vertices as they rest, by the cell of space they lie in.
	type place struct {
		piece *gpuPiece
		at    int
	}

	cells := map[[3]int64][]place{}

	for _, part := range other.parts {
		if !part.own() {
			continue
		}

		for _, piece := range part.pieces {
			positions := piece.resting()
			for _, at := range piece.openVertices() {
				cells[key(positions, at)] = append(cells[key(positions, at)], place{piece, at})
			}
		}
	}

	m.welds = map[*gpuPiece][]weld{}
	joined := 0

	for _, part := range m.parts {
		if !part.own() {
			continue
		}

		for _, piece := range part.pieces {
			positions := piece.resting()

			for _, at := range piece.openVertices() {
				centre := key(positions, at)
				best, nearest := place{}, tolerance*tolerance

				// A vertex near the edge of its cell may be matched by one in
				// the cell beside it.
				for dx := int64(-1); dx <= 1; dx++ {
					for dy := int64(-1); dy <= 1; dy++ {
						for dz := int64(-1); dz <= 1; dz++ {
							for _, candidate := range cells[[3]int64{centre[0] + dx, centre[1] + dy, centre[2] + dz}] {
								theirs := candidate.piece.resting()

								distance := 0.0
								for axis := range 3 {
									step := float64(theirs[candidate.at*3+axis] - positions[at*3+axis])
									distance += step * step
								}

								if distance <= nearest {
									best, nearest = candidate, distance
								}
							}
						}
					}
				}

				if best.piece != nil {
					m.welds[piece] = append(m.welds[piece], weld{vertex: at, from: best.piece, at: best.at})
					joined++
				}
			}
		}
	}

	return joined
}

// resting are the vertices of a skinned piece as its mesh rests, before any
// blend shape or bone moves them, and nothing for a piece that is not
// skinned.
func (p *gpuPiece) resting() []float32 {
	switch {
	case p.skin == nil:
		return nil
	case p.morph != nil:
		return p.morph.basePositions
	default:
		return p.skin.restPositions
	}
}

// applyWelds moves the welded vertices of a piece, which has just been
// posed, to where the vertices they are welded to are drawn now.
func (m *Model) applyWelds(piece *gpuPiece) {
	for _, joined := range m.welds[piece] {
		from := joined.from.skin

		copy(piece.skin.movedPositions[joined.vertex*3:joined.vertex*3+3], from.movedPositions[joined.at*3:joined.at*3+3])
		copy(piece.skin.movedNormals[joined.vertex*3:joined.vertex*3+3], from.movedNormals[joined.at*3:joined.at*3+3])

		// A tangent has four numbers, the fourth the way round its
		// bitangent goes.
		if joined.vertex*4+4 <= len(piece.skin.movedTangents) && joined.at*4+4 <= len(from.movedTangents) {
			copy(piece.skin.movedTangents[joined.vertex*4:joined.vertex*4+4], from.movedTangents[joined.at*4:joined.at*4+4])
		}
	}
}

// own reports whether a part is of its model's own entity, rather than of an
// attachment or an entity that shares its pose.
func (p *gpuPart) own() bool {
	return p.attachment == 0 && !p.sharesPose
}

// openVertices are the vertices of a skinned piece on its open edges, the
// edges only one of its triangles has, where its mesh ends, in the order of
// the vertices.
func (p *gpuPiece) openVertices() []int {
	positions := p.resting()
	if len(positions) == 0 || p.mesh == nil || p.mesh.Indices == nil {
		return nil
	}

	indices := unsafe.Slice(p.mesh.Indices, int(p.mesh.TriangleCount)*3)
	edges := map[[2]uint16]int{}

	for at := 0; at+2 < len(indices); at += 3 {
		for corner := range 3 {
			a, b := indices[at+corner], indices[at+(corner+1)%3]
			if a > b {
				a, b = b, a
			}

			edges[[2]uint16{a, b}]++
		}
	}

	open := map[int]bool{}

	for edge, count := range edges {
		if count == 1 {
			open[int(edge[0])], open[int(edge[1])] = true, true
		}
	}

	vertices := make([]int, 0, len(open))
	for vertex := range open {
		if vertex*3+2 < len(positions) {
			vertices = append(vertices, vertex)
		}
	}

	slices.Sort(vertices)

	return vertices
}
