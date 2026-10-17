package model

import (
	"math"
	"slices"
	"strings"

	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// Placing a whole model is how the games put the entities of a portrait
// together: they load the head and the body each on its own, and attach the
// head where one of its joints meets one of the body's, which the portrait
// settings of a game name rather than any asset file:
//
//	attach = {
//		what = head
//		where = torso
//		joint_attachment = { parent_joint = "bn_sp_thoracic" child_joint = "head_root" }
//	}
//
// Joint finds where a joint rests, and Place moves a model by a transform
// worked out from two of them:
//
//	thoracic, _ := body.Joint("bn_sp_thoracic")
//	root, _ := head.Joint("head_root")
//	undo, _ := root.Inverse()
//	head.Place(undo.Then(thoracic))

// Joint returns where a bone of the model rests, in the pose its mesh is
// stored in: the transform from the bone's own coordinates to the model's,
// in the coordinates of the games' files, as a placement is. It reports
// false when no part of the model is skinned to a bone of that name, or the
// bone's transform cannot be undone.
func (m *Model) Joint(name string) (mat.Transform, bool) {
	for _, part := range m.Parts {
		for _, bone := range part.Skeleton {
			if bone.Name != name {
				continue
			}

			// The file stores the inverse of the bone's resting transform,
			// which undone is where the bone rests.
			undoing, ok := mat.FromMatrix(bone.Transform)
			if !ok {
				return mat.Transform{}, false
			}

			rest, ok := undoing.Inverse()
			if !ok {
				return mat.Transform{}, false
			}

			return rest.Then(part.Placement), true
		}
	}

	return mat.Transform{}, false
}

// Place moves the whole model by a placement in the coordinates of the
// games' files, as if its entities had been placed by it before they were
// converted: their vertices and the directions of their surfaces, the
// offsets of their blend shapes, and the placement of each part, which its
// skin is moved by. The box the model fits in is worked out again.
func (m *Model) Place(placed mat.Transform) {
	if placed.IsIdentity() {
		return
	}

	// The vertices are in the converted coordinates, which are those of the
	// files mirrored across x: placing them is mirroring them back, placing
	// them and mirroring them again.
	converted := Mirror.Then(placed).Then(Mirror)
	normals := converted.Normals()
	turned := converted.Determinant() < 0

	for index := range m.Parts {
		part := &m.Parts[index]
		part.Placement = part.Placement.Then(placed)

		for piece := range part.Pieces {
			part.Pieces[piece].place(converted, normals, turned)
		}
	}

	m.Bounds()
}

// place moves the vertices of a piece by a transform of the converted
// coordinates, of which normals is the transform of its normals. A transform
// that mirrors winds the triangles the other way round, so that their front
// stays in front.
func (p *Piece) place(converted, normals mat.Transform, turned bool) {
	for vertex := 0; vertex+2 < len(p.Positions); vertex += 3 {
		copy(p.Positions[vertex:], narrow(converted.Point(widen(p.Positions[vertex:]))))
	}

	for vertex := 0; vertex+2 < len(p.Normals); vertex += 3 {
		copy(p.Normals[vertex:], narrow(unit(normals.Direction(widen(p.Normals[vertex:])))))
	}

	for vertex := 0; vertex+3 < len(p.Tangents); vertex += 4 {
		copy(p.Tangents[vertex:], narrow(unit(converted.Direction(widen(p.Tangents[vertex:])))))
	}

	// The offsets of a blend shape are directions, which the placement turns
	// but does not move. Those of the normals are turned as normals are; a
	// placement that scales unevenly changes them only approximately.
	for index := range p.BlendShapes {
		shape := &p.BlendShapes[index]

		for vertex := 0; vertex+2 < len(shape.Positions); vertex += 3 {
			copy(shape.Positions[vertex:], narrow(converted.Direction(widen(shape.Positions[vertex:]))))
			copy(shape.Normals[vertex:], narrow(normals.Direction(widen(shape.Normals[vertex:]))))
			copy(shape.Tangents[vertex:], narrow(converted.Direction(widen(shape.Tangents[vertex:]))))
		}
	}

	if !turned {
		return
	}

	for triangle := 0; triangle+2 < len(p.Indices); triangle += 3 {
		p.Indices[triangle+1], p.Indices[triangle+2] = p.Indices[triangle+2], p.Indices[triangle+1]
	}

	for vertex := 3; vertex < len(p.Tangents); vertex += 4 {
		p.Tangents[vertex] = -p.Tangents[vertex]
	}
}

func widen(v []float32) [3]float64 {
	return [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
}

func narrow(v [3]float64) []float32 {
	return []float32{float32(v[0]), float32(v[1]), float32(v[2])}
}

func unit(v [3]float64) [3]float64 {
	length := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if length == 0 {
		return v
	}

	return [3]float64{v[0] / length, v[1] / length, v[2] / length}
}

// SharePose adds the parts of another model to this one, posed with it, the
// way the games draw a portrait accessory with a shared_pose_entity, such as
// the eyes of a head:
//
//	male_eyes_normal = {
//		entity = { entity = "male_eyes_entity" shared_pose_entity = head }
//	}
//
// The other model's meshes are skinned to a skeleton of the same bones as
// this one's. Its parts are placed as this model's first skinned part is, and
// take that part's additive animations, with the attributes that play them,
// so that whatever moves the bones of this model moves them alike. They keep
// their own blend shapes and the attributes that drive those. It reports
// false, and adds nothing, when this model has no skinned part.
//
// The other model is used up: its parts are this model's from then on.
func (m *Model) SharePose(other *Model) bool {
	host := -1

	for index := range m.Parts {
		if len(m.Parts[index].Skeleton) > 0 {
			host = index

			break
		}
	}

	if host < 0 {
		return false
	}

	shared := m.Parts[host]

	var played []Attribute

	for _, attribute := range shared.Attributes {
		if attribute.Animation != "" {
			played = append(played, attribute)
		}
	}

	for index := range other.Parts {
		part := other.Parts[index]

		// The part was placed by its own placement; it is placed again as
		// the shared part is, from where its mesh puts it.
		if undo, ok := part.Placement.Inverse(); ok {
			single := Model{Parts: []Part{part}}
			single.Place(undo.Then(shared.Placement))
			part = single.Parts[0]
		}

		part.Attachment = shared.Attachment

		if len(part.Skeleton) > 0 {
			part.SharesPose = true
			part.Skeleton = sameNames(part.Skeleton, shared.Skeleton)
			part.Additive = shared.Additive
			part.Attributes = append(slices.Clone(played), blendShapes(part.Attributes)...)
		}

		m.Parts = append(m.Parts, part)
	}

	m.Bounds()

	return true
}

// sameNames names the bones of a skeleton as the bones of the skeleton it
// shares a pose with name them. The rigs of the games' hair and beards are
// exported with the namespace of the rig they were made on, so that their
// bones are called male_head_rig_01:bn_h_head where the head's own are
// bn_h_head; the games pose them alike all the same. Everything that moves a
// bone finds it by name, so a bone named with a namespace is renamed to the
// bone without it, where the shared skeleton has one. The skeleton is copied,
// and the one given left as it is.
func sameNames(skeleton, shared []mesh.Bone) []mesh.Bone {
	names := map[string]bool{}
	for _, bone := range shared {
		names[bone.Name] = true
	}

	renamed := slices.Clone(skeleton)

	for index, bone := range renamed {
		if names[bone.Name] {
			continue
		}

		if colon := strings.LastIndexByte(bone.Name, ':'); colon >= 0 && names[bone.Name[colon+1:]] {
			renamed[index].Name = bone.Name[colon+1:]
		}
	}

	return renamed
}

// blendShapes are the attributes of a list that drive a blend shape.
func blendShapes(attributes []Attribute) []Attribute {
	var found []Attribute

	for _, attribute := range attributes {
		if attribute.BlendShape != "" {
			found = append(found, attribute)
		}
	}

	return found
}

// MatchNormals gives the vertices of the model that lie where a vertex of
// another lies the other's normals, within a tolerance, and reports how many
// it changed. It is meant for a model seated on another, as a portrait's head
// is on its body: the two meshes meet in a ring of vertices at the same
// places, which the games' files give normals that disagree by as much as a
// quarter of a right angle, so that the light falls on either side of the
// ring differently and draws it as a seam. Taking the other's normals there
// makes the two surfaces meet smoothly; the model's own normals inside the
// ring are left as they are, and its blend shapes keep moving them.
//
// The models are compared where they rest, as they are placed, before any
// skin moves them.
func (m *Model) MatchNormals(other *Model, tolerance float64) int {
	cell := math.Max(tolerance, 1e-6)
	key := func(position []float32) [3]int64 {
		return [3]int64{int64(math.Floor(float64(position[0]) / cell)), int64(math.Floor(float64(position[1]) / cell)), int64(math.Floor(float64(position[2]) / cell))}
	}

	// The other's vertices, by the cell of space they lie in.
	type vertex struct{ position, normal []float32 }

	cells := map[[3]int64][]vertex{}

	for index := range other.Parts {
		for piece := range other.Parts[index].Pieces {
			found := &other.Parts[index].Pieces[piece]

			for at := 0; at*3+2 < len(found.Positions) && at*3+2 < len(found.Normals); at++ {
				position := found.Positions[at*3 : at*3+3]
				cells[key(position)] = append(cells[key(position)], vertex{position, found.Normals[at*3 : at*3+3]})
			}
		}
	}

	changed := 0

	for index := range m.Parts {
		for piece := range m.Parts[index].Pieces {
			own := &m.Parts[index].Pieces[piece]

			for at := 0; at*3+2 < len(own.Positions) && at*3+2 < len(own.Normals); at++ {
				position := own.Positions[at*3 : at*3+3]
				centre := key(position)

				best, nearest := []float32(nil), tolerance*tolerance

				// A vertex near the edge of its cell may be matched by one in
				// the cell beside it.
				for dx := int64(-1); dx <= 1; dx++ {
					for dy := int64(-1); dy <= 1; dy++ {
						for dz := int64(-1); dz <= 1; dz++ {
							for _, candidate := range cells[[3]int64{centre[0] + dx, centre[1] + dy, centre[2] + dz}] {
								distance := 0.0
								for axis := range 3 {
									step := float64(candidate.position[axis] - position[axis])
									distance += step * step
								}

								if distance <= nearest {
									best, nearest = candidate.normal, distance
								}
							}
						}
					}
				}

				if best != nil {
					copy(own.Normals[at*3:at*3+3], best)
					changed++
				}
			}
		}
	}

	return changed
}
