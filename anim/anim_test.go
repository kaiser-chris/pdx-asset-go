package anim

import (
	"math"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// jointSpec is one joint of a file a test builds.
type jointSpec struct {
	name    string
	changes string
	rest    Pose
	uniform bool
}

// build writes an .anim file the way the games write theirs: the info, with a
// joint per object, then the samples frame by frame.
func build(fps float32, frames int, joints []jointSpec, translations, rotations, scales []float32) []byte {
	writer := meshtest.New()

	writer.Object(1, "info").
		Floats("fps", fps).
		Ints("sa", int32(frames)).
		Ints("j", int32(len(joints)))

	for _, joint := range joints {
		writer.Object(2, joint.name).
			Strings("sa", joint.changes).
			Floats("t", joint.rest.Translation[0], joint.rest.Translation[1], joint.rest.Translation[2]).
			Floats("q", joint.rest.Rotation[0], joint.rest.Rotation[1], joint.rest.Rotation[2], joint.rest.Rotation[3])

		if joint.uniform {
			writer.Floats("s", joint.rest.Scale[0])
		} else {
			writer.Floats("s", joint.rest.Scale[0], joint.rest.Scale[1], joint.rest.Scale[2])
		}
	}

	writer.Object(1, "samples")

	for _, each := range []struct {
		name   string
		values []float32
	}{{"t", translations}, {"q", rotations}, {"s", scales}} {
		if len(each.values) > 0 {
			writer.Floats(each.name, each.values...)
		}
	}

	return writer.Bytes()
}

// still is the rotation that does nothing, written as the files write it.
var still = [4]float32{0, 0, 0, -1}

// twoJoints is a file of two joints over three frames: a root that moves and
// a bone that turns, each leaving the other's channels alone.
func twoJoints() []byte {
	return build(10, 3, []jointSpec{
		{name: "Cart_rig:root", changes: "t", rest: Pose{Translation: [3]float32{1, 2, 3}, Rotation: still, Scale: [3]float32{1, 1, 1}}},
		{name: "wheel", changes: "q", rest: Pose{Rotation: still, Scale: [3]float32{2, 2, 2}}},
	},
		// Three frames of one translation each.
		[]float32{1, 2, 3, 1, 2, 4, 1, 2, 5},
		// Three frames of one rotation each: nothing, a quarter turn
		// around z, and a half turn.
		[]float32{0, 0, 0, 1, 0, 0, 0.70710678, 0.70710678, 0, 0, 1, 0},
		nil,
	)
}

// An animation is read joint by joint, and every frame of a channel its
// samples change is read from the samples, while the channels they leave
// alone stay as the joint's resting pose has them.
func TestReadAnimation(t *testing.T) {
	animation, err := Read(twoJoints())
	if err != nil {
		t.Fatal(err)
	}

	if animation.FPS != 10 || animation.Frames != 3 || len(animation.Joints) != 2 {
		t.Fatalf("%g fps, %d frames, %d joints", animation.FPS, animation.Frames, len(animation.Joints))
	}

	if animation.ScaleWidth != 3 {
		t.Errorf("scale is %d numbers wide, want 3", animation.ScaleWidth)
	}

	if !animation.HasSamples() {
		t.Error("the samples were not read")
	}

	root, wheel := animation.Joints[0], animation.Joints[1]

	if root.Name != "Cart_rig:root" || root.Changes != Translation {
		t.Errorf("the root is %q, changing %v", root.Name, root.Changes)
	}

	if wheel.Changes != Rotation {
		t.Errorf("the wheel changes %v, want the rotation", wheel.Changes)
	}

	if got := animation.Changes(); got != Translation|Rotation {
		t.Errorf("the animation changes %v", got)
	}

	// The root moves along z, and keeps the scale and rotation it rests at.
	for frame, want := range [][3]float32{{1, 2, 3}, {1, 2, 4}, {1, 2, 5}} {
		pose := animation.Pose(0, frame)
		if pose.Translation != want {
			t.Errorf("the root is at %v in frame %d, want %v", pose.Translation, frame, want)
		}

		if pose.Scale != [3]float32{1, 1, 1} || pose.Rotation != still {
			t.Errorf("the root frame %d scales %v and turns %v, want its resting pose", frame, pose.Scale, pose.Rotation)
		}
	}

	// The wheel turns, and stays where it rests, scaled as it rests.
	for frame, want := range [][4]float32{{0, 0, 0, 1}, {0, 0, 0.70710678, 0.70710678}, {0, 0, 1, 0}} {
		pose := animation.Pose(1, frame)
		if pose.Rotation != want {
			t.Errorf("the wheel turns %v in frame %d, want %v", pose.Rotation, frame, want)
		}

		if pose.Scale != [3]float32{2, 2, 2} || pose.Translation != [3]float32{} {
			t.Errorf("the wheel frame %d is at %v scaled %v, want its resting pose", frame, pose.Translation, pose.Scale)
		}
	}

	// A frame or a joint outside the animation.
	if animation.Pose(0, -5).Translation != [3]float32{1, 2, 3} || animation.Pose(0, 99).Translation != [3]float32{1, 2, 5} {
		t.Error("a frame outside the animation was not clamped to its ends")
	}

	if animation.Pose(7, 0) != (Pose{}) {
		t.Error("a joint outside the animation was read")
	}
}

// A file writes fps as its samples over its length, so the length is the
// frames over the fps and the rate it was made at is one frame less.
func TestDurationAndRate(t *testing.T) {
	// Ten seconds of a clip made at fifteen frames a second, which is a
	// hundred and fifty samples, as the games write it.
	animation, err := Read(build(15.100671, 150, []jointSpec{{name: "root", rest: Pose{Rotation: still}}}, nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}

	if got := animation.Duration(); math.Abs(got-9.9333) > 1e-3 {
		t.Errorf("it runs %gs, want 9.9333", got)
	}

	if got := animation.Rate(); math.Abs(got-15) > 1e-3 {
		t.Errorf("it was made at %g frames a second, want 15", got)
	}

	// A pose is one frame, which has no rate.
	one, err := Read(build(30, 1, []jointSpec{{name: "root", rest: Pose{Rotation: still}}}, nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}

	if one.Rate() != 0 {
		t.Errorf("a single frame was made at %g frames a second", one.Rate())
	}
}

// A time between two frames blends them, a looping animation carries its last
// frame back into its first, and one that does not loop holds its ends.
func TestAtBlendsBetweenFrames(t *testing.T) {
	animation, err := Read(twoJoints())
	if err != nil {
		t.Fatal(err)
	}

	// At ten frames a second, frame one is at 0.1s and frame two at 0.2s.
	if got := animation.At(0, 0.15, false).Translation; !near3(got, [3]float32{1, 2, 4.5}) {
		t.Errorf("halfway between the frames the root is at %v, want z of 4.5", got)
	}

	if got := animation.At(0, 0.1, false).Translation; !near3(got, [3]float32{1, 2, 4}) {
		t.Errorf("on the frame the root is at %v", got)
	}

	// Three frames at ten a second run for 0.3s, after which a loop is back
	// where it started.
	if got := animation.At(0, 0.3, true).Translation; !near3(got, [3]float32{1, 2, 3}) {
		t.Errorf("a loop at its length is at %v, want where it started", got)
	}

	if got := animation.At(0, 0.75, true).Translation; !near3(got, [3]float32{1, 2, 4.5}) {
		t.Errorf("a loop two lengths and a half frame on is at %v, want z of 4.5", got)
	}

	// The last frame of a loop runs back into the first over one more
	// interval: halfway from z of 5 to z of 3.
	if got := animation.At(0, 0.25, true).Translation; !near3(got, [3]float32{1, 2, 4}) {
		t.Errorf("between the last frame and the first a loop is at %v, want z of 4", got)
	}

	// Without a loop the ends are held, and time before the start as well.
	if got := animation.At(0, 9, false).Translation; !near3(got, [3]float32{1, 2, 5}) {
		t.Errorf("past its end it is at %v, want its last frame", got)
	}

	if got := animation.At(0, -9, false).Translation; !near3(got, [3]float32{1, 2, 3}) {
		t.Errorf("before its start it is at %v, want its first frame", got)
	}
}

// Rotations are blended along the shorter arc between them, which is the same
// arc for a quaternion and its negative, since both are the same rotation.
func TestBlendingRotations(t *testing.T) {
	quarter := [4]float32{0, 0, 0.70710678, 0.70710678}

	// Halfway from nothing to a quarter turn is an eighth of a turn, whose z
	// and w are the sine and cosine of a sixteenth of a circle.
	eighth := float32(math.Sin(math.Pi / 8))

	blended := slerp([4]float32{0, 0, 0, 1}, quarter, 0.5)
	if math.Abs(float64(blended[2]-eighth)) > 1e-6 {
		t.Errorf("halfway to a quarter turn is %v, want z of %g", blended, eighth)
	}

	// The negative of a quaternion is the same rotation, so blending towards
	// it goes the same way round.
	negated := [4]float32{-quarter[0], -quarter[1], -quarter[2], -quarter[3]}

	other := slerp([4]float32{0, 0, 0, 1}, negated, 0.5)
	if math.Abs(float64(other[2]-blended[2])) > 1e-6 || math.Abs(float64(other[3]-blended[3])) > 1e-6 {
		t.Errorf("blending towards the negated rotation gave %v, want %v", other, blended)
	}

	// Two rotations almost the same, where the arc runs out of precision.
	close := [4]float32{0, 0, 1e-7, 1}
	if got := slerp(close, close, 0.5); math.Abs(float64(got[3]-1)) > 1e-5 {
		t.Errorf("blending a rotation with itself gave %v", got)
	}

	// A rotation of no length at all is read as the one that does nothing.
	if got := slerp([4]float32{}, [4]float32{}, 0.5); got != [4]float32{0, 0, 0, 1} {
		t.Errorf("an empty rotation blended to %v", got)
	}
}

// A head read takes the joints of an animation without its samples, from the
// whole file or from a prefix that stops short of them.
func TestReadHeadLeavesTheSamples(t *testing.T) {
	whole := twoJoints()

	// The samples object starts at its name, which a prefix can stop before.
	cut := strings.Index(string(whole), "samples")
	if cut < 0 {
		t.Fatal("the file holds no samples to cut off")
	}

	for name, data := range map[string][]byte{
		"the whole file": whole,
		"a prefix":       whole[:cut+4],
	} {
		animation, err := ReadHead(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}

		if animation.FPS != 10 || animation.Frames != 3 || len(animation.Joints) != 2 {
			t.Errorf("%s: %g fps, %d frames, %d joints", name, animation.FPS, animation.Frames, len(animation.Joints))
		}

		if animation.HasSamples() {
			t.Errorf("%s: the samples were read", name)
		}

		// Without the samples every frame is the joint's resting pose.
		if got := animation.Pose(0, 2).Translation; got != [3]float32{1, 2, 3} {
			t.Errorf("%s: the root is at %v in frame two, want where it rests", name, got)
		}
	}

	// A prefix that cuts the joints off is reported, rather than read as an
	// animation of fewer joints.
	short := whole[:strings.Index(string(whole), "wheel")]

	if _, err := ReadHead(short); err == nil || !strings.Contains(err.Error(), "cut off") {
		t.Errorf("a prefix without the second joint read as %v", err)
	}
}

// Joints drive the bones of their name, whatever their case and whatever
// namespace the rig was exported under, and a bone no joint names keeps the
// pose the mesh is stored in.
func TestBindMatchesByName(t *testing.T) {
	joints := Joints{
		{Name: "chariot:horse_rig_01:bn_root"},
		{Name: "WHEEL"},
		{Name: "rig:spare"},
	}

	bound := joints.Bind([]string{"bn_root", "wheel", "Hat", "rig:WHEEL"})

	if want := []int{0, 1, -1, 1}; !equal(bound, want) {
		t.Errorf("bound %v, want %v", bound, want)
	}

	if index, ok := joints.Find("Rig:Spare"); !ok || index != 2 {
		t.Errorf("the spare is joint %d, %v", index, ok)
	}

	if _, ok := joints.Find("elbow"); ok {
		t.Error("a bone no joint names was found")
	}

	// A skeleton that repeats a name is driven by the first joint of it.
	repeated := Joints{{Name: "a"}, {Name: "A"}}
	if bound := repeated.Bind([]string{"a"}); !equal(bound, []int{0}) {
		t.Errorf("a repeated name bound %v, want the first joint", bound)
	}
}

// A file that scales every axis alike writes one number, both for the resting
// pose and for every sample.
func TestUniformScale(t *testing.T) {
	data := build(10, 2, []jointSpec{
		{name: "root", changes: "s", rest: Pose{Rotation: still, Scale: [3]float32{1, 1, 1}}, uniform: true},
	}, nil, nil, []float32{1, 4})

	animation, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}

	if animation.ScaleWidth != 1 {
		t.Fatalf("scale is %d numbers wide, want 1", animation.ScaleWidth)
	}

	if got := animation.Pose(0, 1).Scale; got != [3]float32{4, 4, 4} {
		t.Errorf("the second frame scales %v, want four on every axis", got)
	}
}

// A file whose samples do not add up, or that is not an animation at all, is
// refused with what did not add up.
func TestBrokenFiles(t *testing.T) {
	joints := []jointSpec{{name: "root", changes: "t", rest: Pose{Rotation: still}}}

	for name, test := range map[string]struct {
		data []byte
		says string
	}{
		"too few samples": {
			build(10, 3, joints, []float32{1, 2, 3, 4, 5, 6}, nil, nil),
			"holds 6 numbers, not the 9",
		},
		"no samples at all": {
			build(10, 3, joints, nil, nil, nil),
			"has no t",
		},
		"more joints than there are": {
			meshtest.New().Object(1, "info").Floats("fps", 10).Ints("sa", 1).Ints("j", 4).Bytes(),
			"cut off",
		},
		"no info": {
			meshtest.New().Object(1, "samples").Floats("t", 0, 0, 0).Bytes(),
			"has no info",
		},
		"not a mesh file at all": {
			[]byte("this is not a binary file"),
			"does not start with",
		},
	} {
		_, err := Read(test.data)
		if err == nil {
			t.Errorf("%s was read", name)

			continue
		}

		if !strings.Contains(err.Error(), test.says) {
			t.Errorf("%s says %q, want it to mention %q", name, err, test.says)
		}
	}

	// A file that changes nothing needs no samples: the games ship poses.
	pose := build(30, 1, []jointSpec{{name: "root", rest: Pose{Rotation: still, Scale: [3]float32{1, 1, 1}}}}, nil, nil, nil)

	animation, err := Read(pose)
	if err != nil {
		t.Fatalf("a pose that changes nothing: %v", err)
	}

	if animation.Changes() != 0 || !animation.HasSamples() {
		t.Errorf("a pose changes %v", animation.Changes())
	}
}

func TestChannelNames(t *testing.T) {
	for channel, want := range map[Channel]string{
		0:                              "nothing",
		Rotation:                       "rotation",
		Translation | Scale:            "translation, scale",
		Translation | Rotation | Scale: "translation, rotation, scale",
	} {
		if got := channel.String(); got != want {
			t.Errorf("%d is %q, want %q", channel, got, want)
		}
	}
}

func near3(a, b [3]float32) bool {
	for axis := range 3 {
		if math.Abs(float64(a[axis]-b[axis])) > 1e-5 {
			return false
		}
	}

	return true
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}

	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}

	return true
}
