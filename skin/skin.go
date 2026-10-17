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

// Layer is an additive animation laid over the pose of a skeleton, at a time
// of its own, in seconds.
//
// An additive animation moves its joints by how far they are, at that time,
// from where the mesh rests, which is added to whatever pose the skeleton is
// in: the games shape the faces of their portraits that way, one clip for
// each feature, the middle of which is the face as it is stored.
type Layer struct {
	Animation *anim.Animation
	Seconds   float64
}

// Matrices returns the skin matrix of each bone of a skeleton at a time, in
// the order of the skeleton, in the coordinates of the mesh file.
//
// The joints of the animation are matched to the bones by name, as
// anim.Joints.Bind does; a bone no joint drives keeps the pose the mesh is
// stored in. A nil animation leaves every bone there, which makes every
// matrix the identity, unless layers move them.
//
// Each layer is added over the pose, in the order given: a joint it drives is
// moved, in its own coordinates, by the difference between where the layer
// has it and where it rests, before its parent moves it.
func Matrices(animation *anim.Animation, skeleton []mesh.Bone, seconds float64, loop bool, layers ...Layer) []mat.Transform {
	return Pinned(animation, skeleton, seconds, loop, nil, layers...)
}

// Pins hold bones of a skeleton where something else puts them, by name: the
// transform from a bone's own coordinates to those of the mesh file, as a
// bone's resting transform is.
//
// The games attach one skeleton to another that way: the head of a portrait
// follows the joints of its body it is attached by, wherever the body's
// animations take them.
type Pins map[string]mat.Transform

// Pinned is Matrices with some bones pinned where pins say rather than where
// their parents and the animation put them. A layer that drives a pinned bone
// still moves it from there.
//
// A pinned bone moves its own vertices by the whole of its pin, scale and
// all. The bones below it hang from where it is and which way it turns: its
// scale brings them nearer or further, but does not size them. That is how
// the games' joint attachments are taken to work, from Victoria 3: a
// portrait's head is pinned to its body's spine, which an infant's body
// scales to a third. The ring of the neck where the two meet has to shrink
// with the body to meet it, and the neck to shorten with it, while the head
// keeps the size its own animations give it, large for an infant's body as
// the game's portrait editor draws it.
func Pinned(animation *anim.Animation, skeleton []mesh.Bone, seconds float64, loop bool, pins Pins, layers ...Layer) []mat.Transform {
	if len(skeleton) == 0 {
		return nil
	}

	names := make([]string, len(skeleton))
	for index, bone := range skeleton {
		names[index] = bone.Name
	}

	bind := func(animation *anim.Animation) []int {
		if animation != nil {
			return animation.Joints.Bind(names)
		}

		unbound := make([]int, len(skeleton))
		for index := range unbound {
			unbound[index] = -1
		}

		return unbound
	}

	bound := bind(animation)

	layered := make([][]int, len(layers))
	for index, layer := range layers {
		layered[index] = bind(layer.Animation)
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

	// spread is the scale of each pinned bone, which moves the bones below it
	// by as much.
	spread := map[int]float64{}

	for index, bone := range skeleton {
		local := restLocal[index]

		if joint := bound[index]; joint >= 0 {
			local = transformOf(animation.At(joint, seconds, loop))
		}

		// A layer adds how far it has the joint from its rest. Layers that
		// drive the same joint add up, the last given innermost.
		added := mat.Identity()

		for position, layer := range layers {
			joint := layered[position][index]
			if joint < 0 {
				continue
			}

			undoRest, ok := restLocal[index].Inverse()
			if !ok {
				continue
			}

			added = transformOf(layer.Animation.At(joint, layer.Seconds, false)).Then(undoRest).Then(added)
		}

		local = added.Then(local)

		if pin, ok := pins[bone.Name]; ok {
			skins[index] = inverseBind[index].Then(added.Then(pin))

			// The bones below a pinned bone hang from where it is and which
			// way it turns; how much the pin scales it only brings them
			// nearer or further.
			world[index] = added.Then(Rigid(pin))
			spread[index] = math.Cbrt(math.Abs(pin.Determinant()))

			continue
		}

		if parent := bone.Parent; parent >= 0 && parent < len(skeleton) {
			if scale, ok := spread[parent]; ok {
				for axis := range 3 {
					local.Offset[axis] *= scale
				}
			}

			world[index] = local.Then(world[parent])
		} else {
			world[index] = local
		}

		skins[index] = inverseBind[index].Then(world[index])
	}

	return skins
}

// Shared are the skin matrices of a skeleton posed by another's bones, as the
// games pose an entity that shares the pose of another, such as the hair of a
// head: each bone is where the other's bone of the same name is, and moves the
// vertices skinned to it from where its own skeleton has them resting. A bone
// the other skeleton does not have stays where it rests.
//
// Only a bone's own resting place is used, never its parent's, which is what
// lets the hair and the beards of Victoria 3 be posed at all: their files give
// the bones no vertex is skinned to, such as the root and the neck, a matrix
// of zeros, which says nothing of where a bone below them rests.
func Shared(host []mat.Transform, hostSkeleton, skeleton []mesh.Bone) []mat.Transform {
	byName := make(map[string]int, len(hostSkeleton))
	for index, bone := range hostSkeleton {
		byName[bone.Name] = index
	}

	skins := make([]mat.Transform, len(skeleton))

	for index, bone := range skeleton {
		skins[index] = mat.Identity()

		other, ok := byName[bone.Name]
		if !ok || other >= len(host) {
			continue
		}

		// The other's skin matrix undoes its own resting place and puts
		// the bone where it is posed: its resting place is put back, and
		// this bone's undone instead.
		own, ok := mat.FromMatrix(bone.Transform)
		if !ok || own.Determinant() == 0 {
			// A bone of zeros rests where the other's does, as far as
			// anything skinned to it is concerned.
			skins[index] = host[other]

			continue
		}

		hostBind, ok := mat.FromMatrix(hostSkeleton[other].Transform)
		if !ok {
			continue
		}

		hostRest, ok := hostBind.Inverse()
		if !ok {
			continue
		}

		skins[index] = own.Then(hostRest).Then(host[other])
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

// Rigid takes the scale out of where a joint is, leaving where it is and
// which way it turns. A scale that is not the same along every axis is taken
// out as far as its average.
func Rigid(joint mat.Transform) mat.Transform {
	scale := math.Cbrt(math.Abs(joint.Determinant()))
	if scale == 0 {
		return joint
	}

	for row := range 3 {
		for column := range 3 {
			joint.Linear[row][column] /= scale
		}
	}

	return joint
}

// transformOf is the transform of a joint's pose: scaled, then turned and
// moved.
func transformOf(pose anim.Pose) mat.Transform {
	scale := mat.Scale([3]float64{float64(pose.Scale[0]), float64(pose.Scale[1]), float64(pose.Scale[2])})

	return scale.Then(mat.FromQuaternion(pose.Rotation, pose.Translation))
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
