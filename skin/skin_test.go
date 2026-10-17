package skin

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// still is the rotation that does nothing, written as the files write it.
var still = [4]float32{0, 0, 0, -1}

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

// readAnim builds and reads an animation, failing the test if it does not.
func readAnim(t *testing.T, data []byte) *anim.Animation {
	t.Helper()

	animation, err := anim.Read(data)
	if err != nil {
		t.Fatalf("read the animation: %v", err)
	}

	return animation
}

func near(a, b [3]float64) bool {
	for axis := range 3 {
		if math.Abs(a[axis]-b[axis]) > 1e-4 {
			return false
		}
	}

	return true
}

// twoBones is a skeleton of a root and a child five units up from it, in the
// pose the mesh is stored in.
func twoBones() []mesh.Bone {
	return []mesh.Bone{
		{Name: "root", Index: 0, Parent: -1, Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}},
		{Name: "child", Index: 1, Parent: 0, Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, -5, 0}},
	}
}

// A skeleton at its bind pose is moved by identity matrices, whether the
// animation names the joints or there is none at all. This is the test the
// shipped bind_pose animations are checked against.
func TestMatricesIdentityAtBindPose(t *testing.T) {
	skeleton := twoBones()

	animation := readAnim(t, buildAnim(10, 1, []jointSpec{
		{name: "root", rest: anim.Pose{Rotation: still, Scale: [3]float32{1, 1, 1}}},
		{name: "child", rest: anim.Pose{Translation: [3]float32{0, 5, 0}, Rotation: still, Scale: [3]float32{1, 1, 1}}},
	}, nil, nil, nil))

	for index, matrix := range Matrices(animation, skeleton, 0, false) {
		if !matrix.IsIdentity() {
			t.Errorf("bone %d at its bind pose is %+v, want identity", index, matrix)
		}
	}

	// Without an animation every bone is left where it is too.
	for index, matrix := range Matrices(nil, skeleton, 0, false) {
		if !matrix.IsIdentity() {
			t.Errorf("bone %d without an animation is %+v, want identity", index, matrix)
		}
	}
}

// The world of a bone is its own pose followed by that of its parent: a child
// turns with the root it hangs from. Composed the other way round, the child
// would stay put while the root turns, which is how this tells the two apart.
func TestMatricesComposeChildFirst(t *testing.T) {
	skeleton := twoBones()

	// The root turns a quarter around z in the second frame; the child has
	// no samples of its own and keeps its resting translation of five up.
	animation := readAnim(t, buildAnim(10, 2, []jointSpec{
		{name: "root", changes: "q", rest: anim.Pose{Rotation: still, Scale: [3]float32{1, 1, 1}}},
		{name: "child", rest: anim.Pose{Translation: [3]float32{0, 5, 0}, Rotation: still, Scale: [3]float32{1, 1, 1}}},
	},
		nil,
		// Nothing, then a quarter turn around z.
		[]float32{0, 0, 0, 1, 0, 0, 0.70710678, 0.70710678},
		nil,
	))

	matrices := Matrices(animation, skeleton, 0.1, false)

	// The root's own vertex at one along x turns to one along y.
	if got := matrices[0].Point([3]float64{1, 0, 0}); !near(got, [3]float64{0, 1, 0}) {
		t.Errorf("the root turned its vertex to %v, want y", got)
	}

	// The child sits five up in the file; following the turned root it ends
	// five along -x. Parent first would leave it where it was.
	if got := matrices[1].Point([3]float64{0, 5, 0}); !near(got, [3]float64{-5, 0, 0}) {
		t.Errorf("the child follows the root to %v, want -x", got)
	}
}

// A bone no joint names stays in the pose the mesh is stored in, even when a
// sibling moves.
func TestMatricesLeaveUnboundBone(t *testing.T) {
	skeleton := twoBones()

	// Only the child is animated; the root has no joint.
	animation := readAnim(t, buildAnim(10, 2, []jointSpec{
		{name: "child", changes: "t", rest: anim.Pose{Translation: [3]float32{0, 5, 0}, Rotation: still, Scale: [3]float32{1, 1, 1}}},
	},
		[]float32{0, 5, 0, 10, 5, 0},
		nil, nil,
	))

	matrices := Matrices(animation, skeleton, 0.1, false)

	if !matrices[0].IsIdentity() {
		t.Errorf("the unnamed root is %+v, want identity", matrices[0])
	}

	// The child moved ten along x, and its skin matrix carries the vertices
	// it rests on with it.
	if got := matrices[1].Point([3]float64{0, 5, 0}); !near(got, [3]float64{10, 5, 0}) {
		t.Errorf("the child moved to %v, want ten along x", got)
	}
}

// A vertex is moved by the weighted sum of its bones' matrices, and a vertex
// no bone moves is left where it is.
func TestApplyMovesVertices(t *testing.T) {
	moved := mat.Identity()
	moved.Offset = [3]float64{10, 0, 0}

	matrices := []mat.Transform{mat.Identity(), moved}

	positions := []float32{0, 5, 0}
	normals := []float32{0, 1, 0}
	tangents := []float32{1, 0, 0, 1}

	outPositions := make([]float32, len(positions))
	outNormals := make([]float32, len(normals))
	outTangents := make([]float32, len(tangents))

	// The vertex hangs entirely from bone one, which moves ten along x.
	Apply(positions, normals, tangents,
		[]int32{1, 0, -1, -1}, []float32{1, 0, 0, 0}, matrices,
		outPositions, outNormals, outTangents)

	if !near3(outPositions, []float32{10, 5, 0}) {
		t.Errorf("the vertex is at %v, want ten along x", outPositions)
	}

	// Half of two bones is halfway between them.
	Apply(positions, normals, tangents,
		[]int32{0, 1, -1, -1}, []float32{0.5, 0.5, 0, 0}, matrices,
		outPositions, outNormals, outTangents)

	if !near3(outPositions, []float32{5, 5, 0}) {
		t.Errorf("half of two bones put it at %v, want five along x", outPositions)
	}

	// No bone at all leaves it put.
	Apply(positions, normals, tangents,
		[]int32{-1, -1, -1, -1}, []float32{0, 0, 0, 0}, matrices,
		outPositions, outNormals, outTangents)

	if !near3(outPositions, positions) {
		t.Errorf("a vertex no bone moves went to %v", outPositions)
	}
}

// Each vertex is moved by the bones its own skin names, not by those of
// another vertex: a skin read at the wrong offset scrambles the mesh, moving
// one part of a model with another part's bones.
func TestApplyUsesEachVertexsOwnSkin(t *testing.T) {
	// Four vertices, each hanging entirely from a bone of its own, and four
	// bones that carry them ten apart.
	positions := []float32{0, 0, 0, 1, 0, 0, 2, 0, 0, 3, 0, 0}
	normals := []float32{0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1}
	tangents := []float32{1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1}

	joints := []int32{0, -1, -1, -1, 1, -1, -1, -1, 2, -1, -1, -1, 3, -1, -1, -1}
	weights := []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}

	matrices := make([]mat.Transform, 4)
	for bone := range matrices {
		matrices[bone] = mat.Identity()
		matrices[bone].Offset = [3]float64{0, float64(10 * (bone + 1)), 0}
	}

	outPositions := make([]float32, len(positions))
	outNormals := make([]float32, len(normals))
	outTangents := make([]float32, len(tangents))

	Apply(positions, normals, tangents, joints, weights, matrices, outPositions, outNormals, outTangents)

	for vertex := range 4 {
		want := []float32{float32(vertex), float32(10 * (vertex + 1)), 0}

		if !near3(outPositions[vertex*3:], want) {
			t.Errorf("vertex %d is at %v, want %v", vertex, outPositions[vertex*3:vertex*3+3], want)
		}
	}
}

// Normals are turned by the inverse transpose of the skin matrix, and
// tangents by its linear part, so both stay at right angles to the surface.
func TestApplyTurnsNormalsAndTangents(t *testing.T) {
	quarter := mat.FromQuaternion([4]float32{0, 0, 0.70710678, 0.70710678}, [3]float32{})
	matrices := []mat.Transform{quarter}

	positions := []float32{0, 5, 0}
	normals := []float32{0, 1, 0}
	tangents := []float32{1, 0, 0, 1}

	outPositions := make([]float32, len(positions))
	outNormals := make([]float32, len(normals))
	outTangents := make([]float32, len(tangents))

	Apply(positions, normals, tangents,
		[]int32{0, -1, -1, -1}, []float32{1, 0, 0, 0}, matrices,
		outPositions, outNormals, outTangents)

	// A quarter turn around z carries a normal up to -x, and a tangent
	// along x to y.
	if !near3(outNormals, []float32{-1, 0, 0}) {
		t.Errorf("the normal is %v, want -x", outNormals)
	}

	if !near3(outTangents[:3], []float32{0, 1, 0}) {
		t.Errorf("the tangent is %v, want y", outTangents[:3])
	}

	if outTangents[3] != 1 {
		t.Errorf("the bitangent sign is %v, want kept", outTangents[3])
	}
}

func near3(a, b []float32) bool {
	if len(a) < len(b) {
		return false
	}

	for index, value := range b {
		if math.Abs(float64(a[index])-float64(value)) > 1e-4 {
			return false
		}
	}

	return true
}

// layerClip is an additive clip of three frames for the child of twoBones,
// which rests five units up: at its middle frame the child is where it rests,
// at its first one unit to the left, at its last one to the right.
func layerClip(t *testing.T) *anim.Animation {
	t.Helper()

	rest := anim.Pose{Translation: [3]float32{-1, 5, 0}, Rotation: still, Scale: [3]float32{1, 1, 1}}

	return readAnim(t, buildAnim(2, 3, []jointSpec{{name: "child", changes: "t", rest: rest}},
		[]float32{-1, 5, 0, 0, 5, 0, 1, 5, 0}, nil, nil))
}

// A layer moves the joints it drives by how far they are from their rest,
// over whatever pose the skeleton is in, and leaves them where they are at the
// middle of its clip.
func TestMatricesAddLayers(t *testing.T) {
	skeleton := twoBones()
	clip := layerClip(t)
	point := [3]float64{0, 6, 0}

	for _, test := range []struct {
		name    string
		seconds float64
		want    [3]float64
	}{
		{"middle", 0.5, point},
		{"start", 0, [3]float64{-1, 6, 0}},
		{"end", 1, [3]float64{1, 6, 0}},
		{"between", 0.75, [3]float64{0.5, 6, 0}},
	} {
		matrices := Matrices(nil, skeleton, 0, false, Layer{Animation: clip, Seconds: test.seconds})
		if got := matrices[1].Point(point); !near(got, test.want) {
			t.Errorf("%s: the child moves %v to %v, want %v", test.name, point, got, test.want)
		}

		if got := matrices[0].Point(point); !near(got, point) {
			t.Errorf("%s: the root, which no layer drives, moves %v to %v", test.name, point, got)
		}
	}

	// Two layers add up, and a layer is added over the pose of an
	// animation: the root moved up by three carries the child with it.
	lifted := readAnim(t, buildAnim(1, 1, []jointSpec{{name: "root", changes: "t", rest: anim.Pose{Translation: [3]float32{0, 3, 0}, Rotation: still, Scale: [3]float32{1, 1, 1}}}},
		[]float32{0, 3, 0}, nil, nil))

	matrices := Matrices(lifted, skeleton, 0, false, Layer{Animation: clip, Seconds: 1}, Layer{Animation: clip, Seconds: 1})
	if got, want := matrices[1].Point(point), [3]float64{2, 9, 0}; !near(got, want) {
		t.Errorf("two layers over a lifted root move %v to %v, want %v", point, got, want)
	}
}

// A pinned bone is where the pin says, and its children follow it; a layer
// still moves it from there.
func TestPinned(t *testing.T) {
	skeleton := twoBones()
	point := [3]float64{0, 6, 0}

	moved := mat.Identity()
	moved.Offset = [3]float64{10, 0, 0}

	matrices := Pinned(nil, skeleton, 0, false, Pins{"root": moved})
	if got := matrices[1].Point(point); !near(got, [3]float64{10, 6, 0}) {
		t.Errorf("the child of a pinned root moves %v to %v", point, got)
	}

	// The child pinned where it rests stays, whatever its parent does.
	rest := mat.Identity()
	rest.Offset = [3]float64{0, 5, 0}

	matrices = Pinned(nil, skeleton, 0, false, Pins{"root": moved, "child": rest}, Layer{Animation: layerClip(t), Seconds: 1})
	if got := matrices[1].Point(point); !near(got, [3]float64{1, 6, 0}) {
		t.Errorf("a pinned child with a layer moves %v to %v, want the layer's step only", point, got)
	}
}

// A skeleton that shares the pose of another puts each bone where the other's
// bone of the same name is, moving its vertices from where its own skeleton
// rests them; a bone of zeros, as the games' hair rigs give the bones no
// vertex is skinned to, rests where the other's does, and a bone the other
// does not have stays where it is.
func TestShared(t *testing.T) {
	resting := func(y float32) []float32 {
		return []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, -y, 0}
	}

	host := []mesh.Bone{
		{Name: "root", Parent: -1, Transform: resting(0)},
		{Name: "head", Parent: 0, Transform: resting(10)},
	}

	// The host's head is posed five to the side of where it rests at ten.
	posed := mat.Identity()
	posed.Offset = [3]float64{5, 0, 0}
	skins := []mat.Transform{mat.Identity(), posed}

	hair := []mesh.Bone{
		{Name: "root", Parent: -1, Transform: make([]float32, 12)},
		{Name: "head", Parent: 0, Transform: resting(12)},
		{Name: "tail", Parent: 1, Transform: resting(14)},
	}

	shared := Shared(skins, host, hair)

	// A vertex where the hair's head rests goes where the host's head is
	// posed: from twelve up to five across and ten up.
	if got := shared[1].Point([3]float64{0, 12, 0}); math.Abs(got[0]-5) > 1e-9 || math.Abs(got[1]-10) > 1e-9 {
		t.Errorf("the hair's head moves its rest to %v, want 5 10 0", got)
	}

	if shared[0] != skins[0] {
		t.Errorf("the root of zeros = %+v, want the host's", shared[0])
	}

	if !shared[2].IsIdentity() {
		t.Errorf("a bone the host does not have = %+v, want it at rest", shared[2])
	}
}
