package render

import (
	"math"
	"slices"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/skin"
)

// Morphing blends the blend shapes of a piece into its vertices on the CPU,
// the way posing moves them: a piece's vertices change only when the weights
// of its shapes do, which is when the genes of a portrait are changed, so
// there is no need to do it on the GPU every frame. A skinned piece is morphed
// first and posed from its morphed vertices, so that a morphed head turns
// with its bones like the rest.
//
// The additive animations the attributes drive are played the same way: the
// time along each is picked when the attributes change, and the skin lays
// them over the pose; see skin.Layer.

// gpuMorph is the CPU side of a piece with blend shapes.
type gpuMorph struct {
	// base are the vertices of the piece as it was converted, which blending
	// starts from, and shapes what each of its blend shapes moves them by.
	basePositions, baseNormals, baseTangents []float32
	shapes                                   []model.BlendShape

	// weights are how far the piece is morphed into each of its shapes.
	weights []float32

	// positions, normals and tangents are the morphed vertices: the arrays
	// the mesh draws from, or for a skinned piece those its skin poses
	// from.
	positions, normals, tangents []float32
}

// newMorph prepares the morphing of a piece, which starts unmorphed.
func newMorph(piece *model.Piece) *gpuMorph {
	return &gpuMorph{
		basePositions: piece.Positions,
		baseNormals:   piece.Normals,
		baseTangents:  piece.Tangents,
		shapes:        piece.BlendShapes,
		weights:       make([]float32, len(piece.BlendShapes)),
		positions:     clone(piece.Positions),
		normals:       clone(piece.Normals),
		tangents:      clone(piece.Tangents),
	}
}

// blend morphs the vertices by the weights. A normal and the direction of a
// tangent are normalised again, since blending directions shortens them.
func (g *gpuMorph) blend() {
	copy(g.positions, g.basePositions)
	copy(g.normals, g.baseNormals)
	copy(g.tangents, g.baseTangents)

	vertices := len(g.positions) / 3

	for index, shape := range g.shapes {
		weight := g.weights[index]
		if weight == 0 || len(shape.Positions) != vertices*3 {
			continue
		}

		for value := range g.positions {
			g.positions[value] += weight * shape.Positions[value]
			g.normals[value] += weight * shape.Normals[value]
		}

		for vertex := range vertices {
			for axis := range 3 {
				g.tangents[vertex*4+axis] += weight * shape.Tangents[vertex*3+axis]
			}
		}
	}

	for vertex := range vertices {
		normalise(g.normals[vertex*3 : vertex*3+3])
		normalise(g.tangents[vertex*4 : vertex*4+3])
	}
}

// normalise scales a direction to unit length, and leaves one of no length
// as it is.
func normalise(direction []float32) {
	length := float32(math.Sqrt(float64(direction[0]*direction[0] + direction[1]*direction[1] + direction[2]*direction[2])))
	if length == 0 {
		return
	}

	direction[0] /= length
	direction[1] /= length
	direction[2] /= length
}

// play picks the time along each additive animation of a part that the
// values of the attributes driving it give, and reports whether any changed.
// An additive animation no attribute drives is not played.
func (p *gpuPart) play(values map[string]float64) bool {
	if len(p.additive) == 0 {
		return false
	}

	playing := make([]float64, len(p.additive))

	for index, additive := range p.additive {
		value, ok := values[additive.ID]
		if !ok {
			playing[index] = -1

			continue
		}

		playing[index] = additive.At(value)
	}

	if slices.Equal(playing, p.playing) {
		return false
	}

	p.playing = playing

	return true
}

// layers are the additive animations a part plays, at the times picked.
func (p *gpuPart) layers() []skin.Layer {
	var layers []skin.Layer

	for index, seconds := range p.playing {
		if seconds >= 0 && index < len(p.additive) && p.additive[index].Animation != nil {
			layers = append(layers, skin.Layer{Animation: p.additive[index].Animation, Seconds: seconds})
		}
	}

	return layers
}

// Attributes are the names of the attributes that morph the model, in the
// order its parts name them, each once: what Morph takes values for.
func (m *Model) Attributes() []string {
	var names []string

	for index := range m.parts {
		for _, attribute := range m.parts[index].attributes {
			if !slices.Contains(names, attribute.Name) {
				names = append(names, attribute.Name)
			}
		}
	}

	return names
}

// Morph morphs the model by the values of its attributes, by name, such as
// the ones the genes of a portrait's DNA set. Each value is added to the
// attribute's default, as model.Attribute says: each blend shape an attribute
// drives is weighted by the sum, and each additive animation played at the
// place along it the sum picks. Several attributes driving one shape or
// animation add up. An attribute that is not given is at its default, which
// leaves a blend shape out and an additive animation at its middle, so a nil
// map morphs the model back to its mesh, or as near as the middles of its
// additive animations are to where the mesh rests.
//
// It has to run on the thread that owns the OpenGL context, after the model
// has been uploaded. A skinned part is posed again as it last was.
func (m *Model) Morph(attributes map[string]float64) {
	reposed := false

	for index := range m.parts {
		part := &m.parts[index]
		if len(part.attributes) == 0 {
			continue
		}

		weights := map[string]float32{}
		values := map[string]float64{}

		for _, attribute := range part.attributes {
			value := attribute.Default + attributes[attribute.Name]

			if attribute.BlendShape != "" {
				weights[attribute.BlendShape] += float32(value)
			} else {
				values[attribute.Animation] += value
			}
		}

		if part.play(values) {
			reposed = true
		}

		for _, piece := range part.pieces {
			morph := piece.morph
			if morph == nil {
				continue
			}

			changed := false

			for position, shape := range morph.shapes {
				if weight := weights[shape.ID]; morph.weights[position] != weight {
					morph.weights[position] = weight
					changed = true
				}
			}

			if !changed {
				continue
			}

			morph.blend()

			if piece.skin != nil {
				reposed = true

				continue
			}

			rl.UpdateMeshBuffer(*piece.mesh, vertexBufferPosition, floatBytes(morph.positions), 0)
			rl.UpdateMeshBuffer(*piece.mesh, vertexBufferNormal, floatBytes(morph.normals), 0)
			rl.UpdateMeshBuffer(*piece.mesh, vertexBufferTangent, floatBytes(morph.tangents), 0)
		}
	}

	// A model that follows another, or is welded to it, is posed again,
	// since the other may have been morphed.
	if m.follows != nil || m.welded != nil {
		reposed = true
	}

	if reposed {
		m.morphed = true
		m.Pose(m.lastPose, m.lastSeconds, m.lastLoop, slices.Clone(m.lastAttachments))
	}
}
