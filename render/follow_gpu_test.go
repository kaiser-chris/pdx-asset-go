//go:build uitest

package render

import (
	"fmt"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// A model that follows another by its joints moves where the other's
// animation takes those joints, and comes back with it.
func TestFollowMovesWithTheParent(t *testing.T) {
	renderer := withRenderer(t)

	parent, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Unload()

	// The child is the same quad, skinned to bones of other names, each
	// joined to the parent's bone of its vertex.
	built := skinnedQuad(t, meshtest.Quad(2, 2))

	var links []JointLink

	for index := range built.Parts[0].Skeleton {
		name := fmt.Sprintf("child%d", index)
		links = append(links, JointLink{Parent: built.Parts[0].Skeleton[index].Name, Child: name})
		built.Parts[0].Skeleton[index].Name = name
	}

	child, err := renderer.Upload(built)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Unload()

	child.Follow(parent, links...)

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()
	viewer.Frame(child.Min, child.Max)

	draw := func() int {
		rl.BeginDrawing()
		viewer.Draw(child)
		rl.EndDrawing()

		return covered(viewer.Image())
	}

	child.Pose(nil, 0, false, nil)

	rest := draw()
	if rest == 0 {
		t.Fatal("nothing drawn of the child at rest")
	}

	// The parent's animation takes every bone a long way along x; the child,
	// drawn alone, goes with it once posed after it.
	parent.Pose(translationAnimation(t), 0.1, false, []int{0})
	child.Pose(nil, 0, false, nil)

	if count := draw(); count != 0 {
		t.Errorf("%d pixels of the child drawn, want it gone with the parent", count)
	}

	parent.Pose(nil, 0, false, nil)
	child.Morph(nil)

	if count := draw(); count != rest {
		t.Errorf("%d pixels of the child drawn after the parent came back, want %d", count, rest)
	}

	// Let go, the child stays where it rests whatever the parent does.
	child.Follow(nil)
	parent.Pose(translationAnimation(t), 0.1, false, []int{0})
	child.Pose(nil, 0, false, nil)

	if count := draw(); count != rest {
		t.Errorf("%d pixels of the child drawn after letting go, want %d", count, rest)
	}
}

// A joint's position follows the model's pose, in the coordinates it is
// drawn in.
func TestJointPosition(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	if _, _, ok := uploaded.JointPosition("nowhere"); ok {
		t.Error("found a joint that is not there")
	}

	uploaded.Pose(translationAnimation(t), 0.1, false, []int{0})

	// A hundred along x in the files is a hundred the other way drawn.
	rest, posed, ok := uploaded.JointPosition("bone0")
	if !ok || rest != [3]float32{} || posed != [3]float32{-100, 0, 0} {
		t.Errorf("bone0 rests at %v and is at %v, %v", rest, posed, ok)
	}
}

// A joint of a model that follows another takes the other's joint whole, so
// that its own vertices shrink with a parent scaled down, but the bones below
// it hang from it without the scale.
func TestFollowScalesOnlyTheJoint(t *testing.T) {
	renderer := withRenderer(t)

	parent, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Unload()

	// The parent's bones scaled to a quarter.
	var joints []jointSpec
	for bone := range 4 {
		joints = append(joints, jointSpec{name: fmt.Sprintf("bone%d", bone), changes: "s", rest: anim.Pose{Rotation: stillQuat, Scale: [3]float32{0.25, 0.25, 0.25}}})
	}

	quarter := make([]float32, 12)
	for index := range quarter {
		quarter[index] = 0.25
	}

	shrunk, err := anim.Read(buildAnim(1, 1, joints, nil, nil, quarter))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		below bool
	}{{"on the joint", false}, {"below the joint", true}} {
		// The child is a quad all of whose vertices are skinned to one
		// bone: the joint that follows the parent, or a bone below it.
		built := skinnedQuad(t, meshtest.Quad(2, 2))
		skeleton := built.Parts[0].Skeleton
		skeleton[0].Name = "joint"
		skeleton[1].Parent = 0

		drawnBy := int32(0)
		if test.below {
			drawnBy = 1
		}

		for index := range built.Parts[0].Pieces[0].Joints {
			if built.Parts[0].Pieces[0].Joints[index] >= 0 {
				built.Parts[0].Pieces[0].Joints[index] = drawnBy
			}
		}

		child, err := renderer.Upload(built)
		if err != nil {
			t.Fatal(err)
		}

		child.Follow(parent, JointLink{Parent: "bone0", Child: "joint"})

		viewer := renderer.NewViewer(64, 64)
		viewer.Frame(child.Min, child.Max)

		draw := func() int {
			rl.BeginDrawing()
			viewer.Draw(child)
			rl.EndDrawing()

			return covered(viewer.Image())
		}

		parent.Pose(nil, 0, false, nil)
		child.Pose(nil, 0, false, nil)
		rest := draw()

		parent.Pose(shrunk, 0, false, []int{0})
		child.Pose(nil, 0, false, nil)
		count := draw()

		switch {
		case rest == 0:
			t.Errorf("%s: nothing drawn at rest", test.name)
		case test.below && count != rest:
			t.Errorf("%s: %d pixels drawn under a scaled parent, want the %d at rest", test.name, count, rest)
		case !test.below && count*8 > rest:
			t.Errorf("%s: %d pixels drawn under a parent scaled to a quarter, want about a sixteenth of %d", test.name, count, rest)
		}

		viewer.Unload()
		child.Unload()
	}
}
