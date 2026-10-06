// Package skin moves the vertices of a mesh by the bones of its skeleton, the
// step the games' vertex shaders do on the GPU.
//
// An animation drives the joints of a skeleton; each bone gets a skin matrix,
// the matrix that carries a vertex from the pose the mesh is stored in to the
// pose the bone is in now, and each vertex is moved by the weighted sum of up
// to four bones. The arithmetic is the games' row vector convention, the same
// one the mat package holds and the mesh files write.
//
// Everything here is plain Go, so skinning can be checked against the files
// themselves without a GPU. The matrices are computed in the coordinates of
// the mesh file, before it is converted to what a graphics library wants;
// whoever moves a converted mesh must first conjugate them by the transform
// that conversion applied.
package skin

import (
	"math"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// Matrices returns the skin matrix of each bone of a skeleton at a time, in
// the order of the skeleton, in the coordinates of the mesh file.
//
// The joints of the animation are matched to the bones by name, as
// anim.Joints.Bind does; a bone no joint drives keeps the pose the mesh is
// stored in. A nil animation leaves every bone there, which makes every
// matrix the identity.
func Matrices(animation *anim.Animation, skeleton []mesh.Bone, seconds float64, loop bool) []mat.Transform {
	if len(skeleton) == 0 {
		return nil
	}

	names := make([]string, len(skeleton))
	for index, bone := range skeleton {
		names[index] = bone.Name
	}

	var bound []int
	if animation != nil {
		bound = animation.Joints.Bind(names)
	} else {
		bound = make([]int, len(skeleton))
		for index := range bound {
			bound[index] = -1
		}
	}

	// inverseBind is the matrix the file stores for each bone, the inverse
	// of where the bone rests, and restWorld is its resting place, the
	// inverse of that.
	inverseBind := make([]mat.Transform, len(skeleton))
	restWorld := make([]mat.Transform, len(skeleton))

	for index, bone := range skeleton {
		inverseBind[index] = mat.Identity()
		if stored, ok := mat.FromMatrix(bone.Transform); ok {
			inverseBind[index] = stored
		}

		restWorld[index] = mat.Identity()
		if undoing, ok := inverseBind[index].Inverse(); ok {
			restWorld[index] = undoing
		}
	}

	// restLocal is where each bone rests relative to its parent, so that a
	// bone an animation leaves alone still follows its parent's animation.
	restLocal := make([]mat.Transform, len(skeleton))

	for index, bone := range skeleton {
		restLocal[index] = restWorld[index]

		if parent := bone.Parent; parent >= 0 && parent < len(skeleton) {
			if undoing, ok := restWorld[parent].Inverse(); ok {
				restLocal[index] = restWorld[index].Then(undoing)
			}
		}
	}

	world := make([]mat.Transform, len(skeleton))
	skins := make([]mat.Transform, len(skeleton))

	for index, bone := range skeleton {
		local := restLocal[index]

		if joint := bound[index]; joint >= 0 {
			pose := animation.At(joint, seconds, loop)
			scale := mat.Scale([3]float64{float64(pose.Scale[0]), float64(pose.Scale[1]), float64(pose.Scale[2])})
			local = scale.Then(mat.FromQuaternion(pose.Rotation, pose.Translation))
		}

		if parent := bone.Parent; parent >= 0 && parent < len(skeleton) {
			world[index] = local.Then(world[parent])
		} else {
			world[index] = local
		}

		skins[index] = inverseBind[index].Then(world[index])
	}

	return skins
}

// Apply moves the vertices of a skinned piece by their skin matrices, writing
// the result into outPositions, outNormals and outTangents, which must be at
// least as long as the attributes they stand in for.
//
// joints and weights hold mesh.JointsPerVertex bone indices and shares per
// vertex, as a mesh.Skin does; a bone of -1 or a weight of zero leaves that
// influence out. Positions are moved by the weighted sum of the matrices,
// normals by its inverse transpose, and tangents by its linear part, which is
// how the games' shaders move them. A vertex no bone moves is left where it
// is.
func Apply(positions, normals, tangents []float32, joints []int32, weights []float32, matrices []mat.Transform, outPositions, outNormals, outTangents []float32) {
	// vertex indexes a vertex, as the skin does; the position index is
	// three times it, and the tangent index four, so the two must not be
	// confused: a skin read at a position's index drives the wrong bones.
	for vertex := 0; vertex*3+2 < len(positions); vertex++ {
		var blended mat.Transform
		moved := false

		for influence := 0; influence < mesh.JointsPerVertex; influence++ {
			at := vertex*mesh.JointsPerVertex + influence
			if at >= len(joints) || at >= len(weights) {
				break
			}

			bone, weight := joints[at], weights[at]
			if bone < 0 || int(bone) >= len(matrices) || weight == 0 {
				continue
			}

			moved = true
			matrix := matrices[bone]

			for row := range 3 {
				for column := range 3 {
					blended.Linear[row][column] += float64(weight) * matrix.Linear[row][column]
				}

				blended.Offset[row] += float64(weight) * matrix.Offset[row]
			}
		}

		at := vertex * 3

		if !moved {
			copy(outPositions[at:at+3], positions[at:at+3])
		} else {
			movedTo := blended.Point(widen(positions[at:]))
			narrowInto(outPositions[at:at+3], movedTo)
		}

		if at+2 < len(normals) {
			if !moved {
				copy(outNormals[at:at+3], normals[at:at+3])
			} else {
				narrowInto(outNormals[at:at+3], unit(blended.Normals().Direction(widen(normals[at:]))))
			}
		}

		tangentAt := vertex * 4

		if tangentAt+3 < len(tangents) {
			if !moved {
				copy(outTangents[tangentAt:tangentAt+4], tangents[tangentAt:tangentAt+4])
			} else {
				narrowInto(outTangents[tangentAt:tangentAt+3], unit(blended.Direction(widen(tangents[tangentAt:]))))
				outTangents[tangentAt+3] = tangents[tangentAt+3]
			}
		}
	}
}

func widen(values []float32) [3]float64 {
	return [3]float64{float64(values[0]), float64(values[1]), float64(values[2])}
}

func narrowInto(into []float32, from [3]float64) {
	into[0], into[1], into[2] = float32(from[0]), float32(from[1]), float32(from[2])
}

func unit(v [3]float64) [3]float64 {
	length := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if length == 0 {
		return v
	}

	return [3]float64{v[0] / length, v[1] / length, v[2] / length}
}
