package render

import (
	"slices"

	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/skin"
)

// A portrait is several models whose skeletons are joined: the games load the
// head and the body each on its own, and join the head to the body by a few
// of their joints, which the portrait settings of a game name:
//
//	attach = {
//		what = head
//		where = torso
//		joint_attachment = { parent_joint = "bn_sp_thoracic" child_joint = "head_root" }
//		joint_attachment = { parent_joint = "bn_l_clavicle" child_joint = "bn_h_l_clavicle" }
//	}
//
// Each joint of the child named so follows the joint of the parent wherever
// the parent's animations and additive animations take it, keeping the place
// the two had relative to each other where their meshes rest, and the rest of
// the child follows those joints as it follows its own.
//
// A joint of the child takes the parent's whole, scale and all, for its own
// vertices, but the bones below it hang from it without the scale; see
// skin.Pinned for what in Victoria 3 that was read off.

// JointLink joins a joint of a model to a joint of the model it follows.
type JointLink struct {
	Parent, Child string
}

// Follow joins the model to another by joints, which the model's own
// skeleton follows from now on wherever the other's poses take them. A nil
// parent lets go of it again.
//
// Neither model is posed by this. A model that follows another has to be
// posed after it, every time the other is posed or morphed: Morph poses it
// again by itself, and Pose always poses it.
func (m *Model) Follow(parent *Model, links ...JointLink) {
	m.follows = parent
	m.links = slices.Clone(links)
	m.morphed = true
}

// skinOf works out the skin matrices of a part as the model is posed now:
// the animation it was last posed with, if it moves the part, the additive
// animations the attributes play, and the joints of the model it follows. It
// is nil when none of them moves the part, which leaves it where its mesh
// rests.
func (m *Model) skinOf(part *gpuPart) []mat.Transform {
	if len(part.skeleton) == 0 {
		return nil
	}

	// An attachment an animation does not move is put back where its mesh
	// stores it, in case it was moved before: an entity taken off the
	// animation has to stand still, not stay where it was. The additive
	// animations the attributes play are laid over the pose whether or not
	// an animation moves the part.
	played := m.lastPose
	if played != nil && !slices.Contains(m.lastAttachments, part.attachment) {
		played = nil
	}

	layers := part.layers()
	pins := m.pinsOf(part)

	if played == nil && len(layers) == 0 && len(pins) == 0 {
		return nil
	}

	return skin.Pinned(played, part.skeleton, m.lastSeconds, m.lastLoop, pins, layers...)
}

// pinsOf works out where the joints of a part that follow the model's parent
// are, in the coordinates of the part's mesh file.
func (m *Model) pinsOf(part *gpuPart) skin.Pins {
	if m.follows == nil {
		return nil
	}

	undoPlacement, ok := part.placement.Inverse()
	if !ok {
		return nil
	}

	pins := skin.Pins{}

	for _, link := range m.links {
		own, ok := restOf(part, link.Child)
		if !ok {
			continue
		}

		rest, posed, ok := m.follows.joint(link.Parent)
		if !ok {
			continue
		}

		undoRest, ok := rest.Inverse()
		if !ok {
			continue
		}

		// Where the child's joint rests relative to the parent's, carried
		// to where the parent's is now, and taken back into the coordinates
		// of the child's mesh file.
		pins[link.Child] = own.Then(part.placement).Then(undoRest).Then(posed).Then(undoPlacement)
	}

	return pins
}

// joint returns where a joint of the model rests and where it is now, in
// the coordinates of the files with the placement of its part, which are the
// coordinates every model of a portrait shares before it is converted.
func (m *Model) joint(name string) (rest, posed mat.Transform, ok bool) {
	for index := range m.parts {
		part := &m.parts[index]

		at := slices.IndexFunc(part.skeleton, func(bone mesh.Bone) bool { return bone.Name == name })
		if at < 0 {
			continue
		}

		own, ok := restOf(part, name)
		if !ok {
			continue
		}

		rest = own.Then(part.placement)
		posed = rest

		// A skin matrix carries the bone from where it rests to where it is
		// now.
		if matrices := m.skinOf(part); matrices != nil {
			posed = own.Then(matrices[at]).Then(part.placement)
		}

		return rest, posed, true
	}

	return mat.Transform{}, mat.Transform{}, false
}

// restOf returns where a bone of a part rests, in the coordinates of the
// part's mesh file.
func restOf(part *gpuPart, name string) (mat.Transform, bool) {
	for _, bone := range part.skeleton {
		if bone.Name != name {
			continue
		}

		undoing, ok := mat.FromMatrix(bone.Transform)
		if !ok {
			return mat.Transform{}, false
		}

		return undoing.Inverse()
	}

	return mat.Transform{}, false
}

// JointPosition returns where a joint of the model rests and where its poses
// have it now, in the coordinates the model is drawn in, the ones its Min and
// Max are in. The box a model fits in is that of its meshes as they rest; a
// joint shows how far its poses have moved a part of it, such as a head that
// follows a body made taller.
func (m *Model) JointPosition(name string) (rest, posed [3]float32, ok bool) {
	restAt, posedAt, ok := m.joint(name)
	if !ok {
		return rest, posed, false
	}

	// The drawn coordinates are those of the files mirrored across x.
	drawn := func(transform mat.Transform) [3]float32 {
		return [3]float32{float32(-transform.Offset[0]), float32(transform.Offset[1]), float32(transform.Offset[2])}
	}

	return drawn(restAt), drawn(posedAt), true
}
