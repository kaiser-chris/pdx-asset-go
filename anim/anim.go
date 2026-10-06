// Package anim reads the skeletal animations of the games' .anim files.
//
// An animation is a pose for every joint of a skeleton, sampled at a steady
// rate. The .anim files are written in the same binary container as the .mesh
// files, which the mesh package of this module reads, and hold two objects:
//
//	info      fps, how many samples there are and how many joints, then one
//	          object per joint, named after the bone it drives, saying which
//	          of its translation, rotation and scale the samples change, and
//	          the pose it starts in.
//	samples   the values that change, frame by frame, and within a frame in
//	          the order the joints are given.
//
// Every one of the 3651 files the three games ship is laid out that way, with
// no other objects or properties, so there is nothing here to tell versions
// or dialects apart.
//
// # Time
//
// A file writes fps as the number of samples over the length of the
// animation, not as the rate the animation was made at: an animation of 150
// samples written as 15.100671 frames a second is ten seconds of a clip made
// at 15 frames a second, whose last sample closes the loop. So Duration is
// Frames over FPS, and frame n is at n/FPS, which gives a whole rate of 15,
// 24, 25, 30 or 60 for 2897 of the files the games ship, against 38 for
// reading it the other way round.
//
// # Driving a skeleton
//
// The joints of an animation are matched to the bones of a mesh by name,
// whatever their case and without the namespace a rig is exported under:
// "zeppelin_01_rig:root" drives the bone "root". Matched that way, every one
// of the 7687 pairs of mesh and animation the three games ship lines up but
// eighty odd, which share animations between skeletons that differ. See
// Joints.Bind.
//
// A joint's pose is in the space of the bone's parent. Walking the bones of a
// skeleton in order, the pose of a bone in the space of the mesh is its own
// pose followed by that of its parent, and the matrix that moves a vertex
// from the pose the mesh is stored in is the bone's Transform, which is the
// inverse of its resting pose, followed by that. Composed the other way round
// the shipped bind poses are out by up to 200 units, against a millionth of
// one composed this way, on rigs eleven bones deep.
package anim

import (
	"fmt"
	"math"
	"strings"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// HeadBytes is enough of a file for ReadHead to read its info. The largest
// rig the three games ship, of 134 bones, writes a sixth of this, and one
// body of Crusader Kings 3 has its head read seven hundred times over when
// its animations are listed, so it is kept no larger than it needs to be.
const HeadBytes = 64 << 10

// MaxJoints bounds how many joints a file may claim, far above the 134 of the
// largest rig the games ship, so that a broken count cannot ask for memory.
const MaxJoints = 4096

// Channel is one of the three things an animation can change about a joint.
type Channel uint8

const (
	Translation Channel = 1 << iota
	Rotation
	Scale
)

func (c Channel) String() string {
	var named []string

	for _, each := range []struct {
		Channel
		name string
	}{{Translation, "translation"}, {Rotation, "rotation"}, {Scale, "scale"}} {
		if c&each.Channel != 0 {
			named = append(named, each.name)
		}
	}

	if len(named) == 0 {
		return "nothing"
	}

	return strings.Join(named, ", ")
}

// Pose is where a joint is, in the space of its parent.
type Pose struct {
	Translation [3]float32

	// Rotation is a quaternion of x, y, z and w. The files write the
	// rotation that does nothing as w = -1 as often as w = 1, which is the
	// same rotation.
	Rotation [4]float32

	// Scale is per axis. A file that scales every axis alike writes one
	// number, which is read into all three.
	Scale [3]float32
}

// Joint is one joint of an animation, which drives the bone of its name.
type Joint struct {
	Name string

	// Changes are the channels the samples change for this joint. What is
	// not among them stays as Rest has it for the whole animation.
	Changes Channel

	// Rest is the pose the joint starts in, which is also its pose in every
	// frame for the channels the samples leave alone.
	Rest Pose

	// Where within one frame of each sample array this joint's values are,
	// counted in values rather than joints, or -1 for a channel the samples
	// leave alone.
	translation int
	rotation    int
	scale       int
}

// Animation is one .anim file.
type Animation struct {
	// FPS is the number of samples over the length of the animation, as the
	// file writes it. Duration says how to read it.
	FPS float32

	// Frames is how many samples there are. A file may hold one, which is a
	// pose rather than an animation.
	Frames int

	// Joints are the joints of the skeleton the animation was made for, in
	// the order the samples give their values in.
	Joints Joints

	// ScaleWidth is how many numbers a scale sample holds: three, or one for
	// a file that scales every axis alike.
	ScaleWidth int

	// The sampled values, frame by frame. A head read holds none of them.
	translations []float32
	rotations    []float32
	scales       []float32

	// The width of one frame of each array.
	strides [3]int
}

// Joints are the joints of an animation.
type Joints []Joint

// Duration is how long the animation runs, in seconds: its frames over its
// FPS, as the package comment explains. Frame n is at n/FPS, and the last
// frame runs for one more interval, which a loop plays back into the first.
func (a *Animation) Duration() float64 {
	if a.FPS <= 0 {
		return 0
	}

	return float64(a.Frames) / float64(a.FPS)
}

// Rate is the rate the animation was made at: the rate at which its frames
// are a frame apart, which is one below its samples over its length.
func (a *Animation) Rate() float64 {
	if a.Frames < 2 {
		return 0
	}

	return float64(a.Frames-1) / a.Duration()
}

// HasSamples reports whether the animation holds its samples, which a head
// read leaves out.
func (a *Animation) HasSamples() bool {
	return len(a.translations)+len(a.rotations)+len(a.scales) > 0 || a.Changes() == 0
}

// Changes are the channels the animation changes for any of its joints.
func (a *Animation) Changes() Channel {
	var changed Channel

	for _, joint := range a.Joints {
		changed |= joint.Changes
	}

	return changed
}

// Pose is where a joint is at one frame. A frame outside the animation is
// clamped to its ends, and a joint outside it is the zero Pose. Without the
// samples, as after a head read, every frame is the joint's resting pose.
func (a *Animation) Pose(joint, frame int) Pose {
	if joint < 0 || joint >= len(a.Joints) {
		return Pose{}
	}

	at := a.Joints[joint]
	pose := at.Rest

	if a.Frames == 0 {
		return pose
	}

	frame = min(max(frame, 0), a.Frames-1)

	if values, ok := a.frame(a.translations, a.strides[0], at.translation, frame, 3); ok {
		pose.Translation = [3]float32{values[0], values[1], values[2]}
	}

	if values, ok := a.frame(a.rotations, a.strides[1], at.rotation, frame, 4); ok {
		pose.Rotation = [4]float32{values[0], values[1], values[2], values[3]}
	}

	if values, ok := a.frame(a.scales, a.strides[2], at.scale, frame, a.ScaleWidth); ok {
		pose.Scale = [3]float32{values[0], values[0], values[0]}
		if len(values) == 3 {
			pose.Scale = [3]float32{values[0], values[1], values[2]}
		}
	}

	return pose
}

// frame returns one joint's values of one frame of a sample array.
func (a *Animation) frame(values []float32, stride, within, frame, width int) ([]float32, bool) {
	if within < 0 || width <= 0 || stride <= 0 {
		return nil, false
	}

	start := frame*stride + within
	if start < 0 || start+width > len(values) {
		return nil, false
	}

	return values[start : start+width], true
}

// At is where a joint is at a time in seconds, between the frames it falls
// between. An animation that loops carries the last frame back into the
// first over the interval after it; one that does not holds its last frame.
//
// Which way the engine blends its samples is not known from the files, which
// hold no more than the samples themselves. This blends them the plain way:
// along the line between two translations or scales, and along the shorter
// arc between two rotations.
func (a *Animation) At(joint int, seconds float64, loop bool) Pose {
	if a.Frames <= 1 || a.FPS <= 0 {
		return a.Pose(joint, 0)
	}

	position := seconds * float64(a.FPS)

	var next int

	switch {
	case loop:
		length := float64(a.Frames)
		position -= length * math.Floor(position/length)
	case position <= 0:
		return a.Pose(joint, 0)
	case position >= float64(a.Frames-1):
		return a.Pose(joint, a.Frames-1)
	}

	frame := int(math.Floor(position))
	blend := position - float64(frame)

	if loop {
		frame %= a.Frames
		next = (frame + 1) % a.Frames
	} else {
		next = frame + 1
	}

	return blendPoses(a.Pose(joint, frame), a.Pose(joint, next), float32(blend))
}

// blendPoses blends two poses, by how far between them to go.
func blendPoses(from, to Pose, how float32) Pose {
	blended := Pose{Rotation: slerp(from.Rotation, to.Rotation, how)}

	for axis := range 3 {
		blended.Translation[axis] = from.Translation[axis] + (to.Translation[axis]-from.Translation[axis])*how
		blended.Scale[axis] = from.Scale[axis] + (to.Scale[axis]-from.Scale[axis])*how
	}

	return blended
}

// slerp blends two rotations along the shorter arc between them. A quaternion
// and its negative are the same rotation, so one of them is turned round when
// that is the shorter way; two rotations almost the same are blended along
// the line between them, which is where the arc runs out of precision.
func slerp(from, to [4]float32, how float32) [4]float32 {
	a, b := unit4(from), unit4(to)

	dot := 0.0
	for axis := range 4 {
		dot += a[axis] * b[axis]
	}

	if dot < 0 {
		for axis := range 4 {
			b[axis] = -b[axis]
		}

		dot = -dot
	}

	factorA, factorB := 1-float64(how), float64(how)

	if dot < 0.9995 {
		angle := math.Acos(min(dot, 1))
		if sin := math.Sin(angle); sin > 1e-9 {
			factorA = math.Sin(float64(1-how)*angle) / sin
			factorB = math.Sin(float64(how)*angle) / sin
		}
	}

	var blended [4]float64
	for axis := range 4 {
		blended[axis] = a[axis]*factorA + b[axis]*factorB
	}

	return narrow4(blended)
}

// unit4 is a quaternion of unit length, as float64. One of no length at all
// is read as the rotation that does nothing.
func unit4(q [4]float32) [4]float64 {
	wide := [4]float64{float64(q[0]), float64(q[1]), float64(q[2]), float64(q[3])}

	length := math.Sqrt(wide[0]*wide[0] + wide[1]*wide[1] + wide[2]*wide[2] + wide[3]*wide[3])
	if length == 0 {
		return [4]float64{0, 0, 0, 1}
	}

	for axis := range 4 {
		wide[axis] /= length
	}

	return wide
}

func narrow4(q [4]float64) [4]float32 {
	length := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if length == 0 {
		return [4]float32{0, 0, 0, 1}
	}

	var narrowed [4]float32
	for axis := range 4 {
		narrowed[axis] = float32(q[axis] / length)
	}

	return narrowed
}

// Find returns the joint that drives a bone of the given name, matched the way
// Bind matches them.
func (j Joints) Find(bone string) (int, bool) {
	wanted := bare(bone)

	for index, joint := range j {
		if bare(joint.Name) == wanted {
			return index, true
		}
	}

	return -1, false
}

// Bind matches the joints of an animation to the bones of a skeleton by name,
// returning the joint that drives each bone in turn, or -1 for a bone the
// animation leaves in the pose the mesh is stored in.
//
// Names are matched whatever their case and without the namespace the rig was
// exported under, the part before the last colon: the joint
// "chariot_export:horse_rig_01:bn_root" drives the bone "bn_root". The
// shipped animations hold a joint for every bone of the skeleton they were
// made for, in the same order, but the games also share one skeleton's
// animations with another through a skeletal_animation_set, and a few
// animations carry a root the mesh has no bone for, so neither the order nor
// the count can be relied on.
func (j Joints) Bind(bones []string) []int {
	byName := make(map[string]int, len(j))

	// The first joint of a name wins, as a skeleton that repeats a name is
	// driven by the first that matches either way.
	for index, joint := range j {
		if name := bare(joint.Name); name != "" {
			if _, taken := byName[name]; !taken {
				byName[name] = index
			}
		}
	}

	bound := make([]int, len(bones))

	for index, bone := range bones {
		joint, ok := byName[bare(bone)]
		if !ok {
			joint = -1
		}

		bound[index] = joint
	}

	return bound
}

// bare is a joint or bone name without the namespace it was exported under,
// and in one case, for matching.
func bare(name string) string {
	if colon := strings.LastIndex(name, ":"); colon >= 0 {
		name = name[colon+1:]
	}

	return strings.ToLower(name)
}

// Read reads an animation from a whole .anim file.
func Read(data []byte) (*Animation, error) {
	root, err := mesh.Decode(data)
	if err != nil {
		return nil, err
	}

	return read(root, true)
}

// ReadHead reads everything but the samples of an animation, from a whole
// file or from its first HeadBytes bytes, which is enough for every rig the
// games ship. It is for listing the animations of a mesh without reading
// their samples, which for one body of Crusader Kings 3 run to hundreds of
// megabytes across its animations.
//
// A prefix too short for the info of a large rig is reported as a file whose
// joints are cut off, so the whole file can be read instead.
func ReadHead(data []byte) (*Animation, error) {
	root, err := mesh.DecodeHead(data)
	if err != nil {
		return nil, err
	}

	return read(root, false)
}

// The names of the objects and properties of a .anim file.
const (
	objectInfo    = "info"
	objectSamples = "samples"

	propertyFPS         = "fps"
	propertyFrames      = "sa"
	propertyJoints      = "j"
	propertyChanges     = "sa"
	propertyTranslation = "t"
	propertyRotation    = "q"
	propertyScale       = "s"
)

// read reads a decoded file, with its samples or without them.
func read(root *mesh.Object, samples bool) (*Animation, error) {
	info, ok := root.Child(objectInfo)
	if !ok {
		return nil, fmt.Errorf("not an animation: it has no %s", objectInfo)
	}

	fps, ok := info.Property(propertyFPS)
	if !ok || len(fps.Floats) != 1 {
		return nil, fmt.Errorf("%s has no one %s", objectInfo, propertyFPS)
	}

	frames, ok := whole(info, propertyFrames)
	if !ok {
		return nil, fmt.Errorf("%s has no one %s, how many samples there are", objectInfo, propertyFrames)
	}

	count, ok := whole(info, propertyJoints)
	if !ok {
		return nil, fmt.Errorf("%s has no one %s, how many joints there are", objectInfo, propertyJoints)
	}

	switch {
	case frames < 0:
		return nil, fmt.Errorf("%s says there are %d samples", objectInfo, frames)
	case count < 0 || count > MaxJoints:
		return nil, fmt.Errorf("%s says there are %d joints, more than the %d this reads", objectInfo, count, MaxJoints)
	case len(info.Children) < count:
		return nil, fmt.Errorf("%s says there are %d joints but holds %d, so the file is cut off", objectInfo, count, len(info.Children))
	}

	animation := &Animation{FPS: fps.Floats[0], Frames: frames, ScaleWidth: 1}

	// The width of a scale sample is the width the joints write their
	// resting scale at, which every file the games ship keeps to.
	for _, joint := range info.Children[:count] {
		if scale, ok := joint.Property(propertyScale); ok && len(scale.Floats) == 3 {
			animation.ScaleWidth = 3

			break
		}
	}

	for _, joint := range info.Children[:count] {
		animation.Joints = append(animation.Joints, readJoint(joint, animation))
	}

	if !samples {
		return animation, nil
	}

	return animation, readSamples(root, animation)
}

// readJoint reads one joint of the info, and counts its place in a frame of
// the samples.
func readJoint(object *mesh.Object, into *Animation) Joint {
	joint := Joint{Name: object.Name, translation: -1, rotation: -1, scale: -1}

	if values, ok := object.Property(propertyTranslation); ok && len(values.Floats) >= 3 {
		joint.Rest.Translation = [3]float32{values.Floats[0], values.Floats[1], values.Floats[2]}
	}

	if values, ok := object.Property(propertyRotation); ok && len(values.Floats) >= 4 {
		joint.Rest.Rotation = [4]float32{values.Floats[0], values.Floats[1], values.Floats[2], values.Floats[3]}
	} else {
		joint.Rest.Rotation = [4]float32{0, 0, 0, 1}
	}

	joint.Rest.Scale = [3]float32{1, 1, 1}

	if values, ok := object.Property(propertyScale); ok {
		switch len(values.Floats) {
		case 1:
			joint.Rest.Scale = [3]float32{values.Floats[0], values.Floats[0], values.Floats[0]}
		case 3:
			joint.Rest.Scale = [3]float32{values.Floats[0], values.Floats[1], values.Floats[2]}
		}
	}

	// Which channels the samples change is written as the letters of those
	// channels, together in one string.
	if changes, ok := object.Property(propertyChanges); ok && len(changes.Strings) == 1 {
		for _, letter := range changes.Strings[0] {
			switch letter {
			case 't':
				joint.Changes |= Translation
				joint.translation = into.strides[0]
				into.strides[0] += 3
			case 'q':
				joint.Changes |= Rotation
				joint.rotation = into.strides[1]
				into.strides[1] += 4
			case 's':
				joint.Changes |= Scale
				joint.scale = into.strides[2]
				into.strides[2] += into.ScaleWidth
			}
		}
	}

	return joint
}

// readSamples reads the samples, holding each array to the width the joints
// that change that channel need, times the number of frames.
func readSamples(root *mesh.Object, into *Animation) error {
	object, ok := root.Child(objectSamples)
	if !ok {
		if into.Changes() == 0 {
			// Nothing changes, so there is nothing to sample: a file that
			// writes no samples at all is a pose, which the games ship.
			return nil
		}

		return fmt.Errorf("the animation changes its %v but has no %s", into.Changes(), objectSamples)
	}

	for index, each := range []struct {
		name string
		into *[]float32
	}{
		{propertyTranslation, &into.translations},
		{propertyRotation, &into.rotations},
		{propertyScale, &into.scales},
	} {
		stride := into.strides[index]
		if stride == 0 {
			continue
		}

		values, ok := object.Property(each.name)
		if !ok {
			return fmt.Errorf("%s has no %s, which %d numbers of every frame need", objectSamples, each.name, stride)
		}

		if want := stride * into.Frames; len(values.Floats) != want {
			return fmt.Errorf("%s %s holds %d numbers, not the %d of %d frames of %d",
				objectSamples, each.name, len(values.Floats), want, into.Frames, stride)
		}

		*each.into = values.Floats
	}

	return nil
}

// whole reads a property of one whole number.
func whole(object *mesh.Object, name string) (int, bool) {
	property, ok := object.Property(name)
	if !ok || len(property.Ints) != 1 {
		return 0, false
	}

	return int(property.Ints[0]), true
}
