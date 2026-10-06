//go:build uitest

package render

import (
	"fmt"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// still is the rotation that does nothing, written as the files write it.
var stillQuat = [4]float32{0, 0, 0, -1}

// jointSpec is one joint of an animation a test builds.
type jointSpec struct {
	name    string
	changes string
	rest    anim.Pose
}

// buildAnim writes an .anim file the way the games write theirs.
func buildAnim(fps float32, frames int, joints []jointSpec, translations, rotations, scales []float32) []byte {
	writer := meshtest.New()

	writer.Object(1, "info").
		Floats("fps", fps).
		Ints("sa", int32(frames)).
		Ints("j", int32(len(joints)))

	for _, joint := range joints {
		writer.Object(2, joint.name).
			Strings("sa", joint.changes).
			Floats("t", joint.rest.Translation[0], joint.rest.Translation[1], joint.rest.Translation[2]).
			Floats("q", joint.rest.Rotation[0], joint.rest.Rotation[1], joint.rest.Rotation[2], joint.rest.Rotation[3]).
			Floats("s", joint.rest.Scale[0], joint.rest.Scale[1], joint.rest.Scale[2])
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

// skinnedQuad is a red quad moved by a single bone at the origin.
func skinnedQuad(t *testing.T, geometry meshtest.Geometry) *model.Model {
	t.Helper()

	file, err := mesh.Read(meshtest.New().Object(1, "object").Object(2, "s").Mesh(geometry, "").Bytes())
	if err != nil {
		t.Fatal(err)
	}

	source := &file.Shapes[0].Meshes[0]

	source.Skin = &mesh.Skin{
		Influences: 1,
		Joints:     make([]int32, source.Vertices()*mesh.JointsPerVertex),
		Weights:    make([]float32, source.Vertices()*mesh.JointsPerVertex),
	}

	for vertex := range source.Vertices() {
		at := vertex * mesh.JointsPerVertex
		source.Skin.Joints[at] = int32(vertex)
		source.Skin.Weights[at] = 1

		for influence := 1; influence < mesh.JointsPerVertex; influence++ {
			source.Skin.Joints[at+influence] = -1
		}
	}

	// A bone of its own for every vertex, each at the origin, so that a
	// vertex driven by another vertex's bone shows up: the quad would be
	// torn rather than moved whole.
	skeleton := make([]mesh.Bone, source.Vertices())

	for bone := range skeleton {
		skeleton[bone] = mesh.Bone{Name: fmt.Sprintf("bone%d", bone), Index: bone, Parent: -1, Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}}
	}

	built := &model.Model{
		Name: "skinned",
		Parts: []model.Part{{
			Name:      "quad",
			Pieces:    model.Convert(source),
			Skeleton:  skeleton,
			Placement: mat.Identity(),
			Textures:  model.Textures{Diffuse: &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: []byte{255, 0, 0, 255}}},
		}},
	}
	built.Bounds()

	return built
}

// translationAnimation moves every bone of the quad a long way along x in the
// second frame.
func translationAnimation(t *testing.T) *anim.Animation {
	t.Helper()

	var joints []jointSpec
	for bone := range 4 {
		joints = append(joints, jointSpec{name: fmt.Sprintf("bone%d", bone), changes: "t", rest: anim.Pose{Rotation: stillQuat, Scale: [3]float32{1, 1, 1}}})
	}

	data := buildAnim(10, 2, joints,
		// A frame of the four bones at rest, then a frame of them all a long
		// way along x.
		[]float32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 100, 0, 0, 100, 0, 0, 100, 0, 0},
		nil, nil,
	)

	animation, err := anim.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	return animation
}

// A skinned model moves when it is posed: the quad leaves the picture, and
// comes back when posed at its bind pose again.
func TestPoseMovesSkinnedGeometry(t *testing.T) {
	renderer := withRenderer(t)
	animation := translationAnimation(t)

	uploaded, err := renderer.Upload(skinnedQuad(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()
	viewer.Frame(uploaded.Min, uploaded.Max)

	draw := func() {
		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()
	}

	draw()
	if count := covered(viewer.Image()); count == 0 {
		t.Fatal("nothing drawn of the quad at rest")
	}

	// A long way along x in the file is a long way to the left once
	// converted, off the picture the camera still frames at the quad's rest.
	// Every vertex of the quad has a bone of its own, so every one of them
	// moving leaves nothing drawn: a vertex driven by another's bone would
	// leave part of the quad behind.
	uploaded.Pose(animation, 0.1, false, []int{0})
	draw()
	if count := covered(viewer.Image()); count != 0 {
		t.Errorf("%d pixels still drawn after posing, want the whole quad gone", count)
	}

	// The quad is the model's own entity, attachment zero: an animation that
	// does not name it, or names nothing, leaves it where its mesh stores it.
	uploaded.Pose(animation, 0.1, false, []int{1})
	draw()
	if count := covered(viewer.Image()); count == 0 {
		t.Error("the quad did not come back when its attachment was left out")
	}

	// Posed with no animation, it is back where it started.
	uploaded.Pose(nil, 0, false, nil)
	draw()
	if count := covered(viewer.Image()); count == 0 {
		t.Error("the quad did not come back at the bind pose")
	}
}
