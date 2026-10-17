package model

import (
	"math"
	"slices"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// Placing a converted model moves it as placing its mesh before converting it
// would have: a step along x in the files is a step the other way once
// converted.
func TestPlace(t *testing.T) {
	quad := quadMesh(t)

	shape := *quad
	shape.Positions = append([]float32(nil), quad.Positions...)
	shape.Positions[0] = -3

	// A bone at 5 along x in the files, whose stored transform undoes that.
	bone := mesh.Bone{Name: "root", Parent: -1, Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, -5, 0, 0}}

	built := &Model{Parts: []Part{{
		Pieces:    Convert(quad, Shape{ID: "wide", Mesh: &shape}),
		Skeleton:  []mesh.Bone{bone},
		Placement: mat.Identity(),
	}}}
	built.Bounds()

	if joint, ok := built.Joint("root"); !ok || joint.Offset != [3]float64{5, 0, 0} {
		t.Fatalf("joint = %+v, %v", joint, ok)
	}

	if _, ok := built.Joint("elsewhere"); ok {
		t.Error("found a joint that is not there")
	}

	// Turned a quarter round y, then moved up: the quad faced the camera,
	// along z, and now faces along x.
	placed := mat.Turning(1, math.Pi/2)
	placed.Offset = [3]float64{0, 10, 0}

	built.Place(placed)

	if joint, _ := built.Joint("root"); !near(joint.Offset, [3]float64{0, 10, -5}) {
		t.Errorf("the joint is at %v after placing, want 0 10 -5", joint.Offset)
	}

	// Every vertex is where placing the mesh and converting it puts it.
	want := Convert(placedMesh(quad, placed))[0]
	piece := built.Parts[0].Pieces[0]

	for index := range piece.Positions {
		if math.Abs(float64(piece.Positions[index]-want.Positions[index])) > 1e-5 ||
			math.Abs(float64(piece.Normals[index]-want.Normals[index])) > 1e-5 {
			t.Fatalf("value %d at %g and normal %g, want %g and %g", index, piece.Positions[index], piece.Normals[index], want.Positions[index], want.Normals[index])
		}
	}

	// The offset of the blend shape turned with it: the first corner went
	// from -1 to -3 along x, which turned is along z, and converted the same.
	offset := vec(piece.BlendShapes[0].Positions, 0, 3)
	if !near([3]float64{float64(offset[0]), float64(offset[1]), float64(offset[2])}, [3]float64{0, 0, 2}) {
		t.Errorf("the blend shape moves the first corner by %v, want 2 along z", offset)
	}

	if built.Parts[0].Placement.Offset != placed.Offset || built.Min[1] != 9 || built.Max[1] != 11 {
		t.Errorf("placement %+v, bounds %v to %v", built.Parts[0].Placement, built.Min, built.Max)
	}
}

// placedMesh places the vertices of a mesh, which is what Place has to agree
// with.
func placedMesh(source *mesh.Mesh, placed mat.Transform) *mesh.Mesh {
	copied := *source
	copied.Positions = make([]float32, len(source.Positions))
	copied.Normals = make([]float32, len(source.Normals))

	for vertex := 0; vertex+2 < len(source.Positions); vertex += 3 {
		copy(copied.Positions[vertex:], narrow(placed.Point(widen(source.Positions[vertex:]))))
		copy(copied.Normals[vertex:], narrow(placed.Normals().Direction(widen(source.Normals[vertex:]))))
	}

	return &copied
}

func near(a, b [3]float64) bool {
	for axis := range 3 {
		if math.Abs(a[axis]-b[axis]) > 1e-5 {
			return false
		}
	}

	return true
}

// A model that shares the pose of another is placed as it is, and takes its
// additive animations and the attributes that play them, keeping its own
// blend shapes.
func TestSharePose(t *testing.T) {
	bone := mesh.Bone{Name: "root", Parent: -1, Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}}
	slide := Additive{ID: "slide"}

	head := &Model{Parts: []Part{
		{Name: "unskinned", Placement: mat.Identity()},
		{
			Name:       "head",
			Pieces:     Convert(quadMesh(t)),
			Skeleton:   []mesh.Bone{bone},
			Placement:  mat.Identity(),
			Attachment: 2,
			Additive:   []Additive{slide},
			Attributes: []Attribute{{Name: "slide_far", Animation: "slide", Default: 0.5}, {Name: "fat", BlendShape: "bs_fat"}},
		},
	}}

	placed := mat.Identity()
	placed.Offset = [3]float64{0, 10, 0}
	head.Place(placed)

	eyes := &Model{Parts: []Part{{
		Name:       "eyes",
		Pieces:     Convert(quadMesh(t)),
		Skeleton:   []mesh.Bone{bone},
		Placement:  mat.Identity(),
		Attributes: []Attribute{{Name: "big_eyes", BlendShape: "bs_big"}},
	}}}

	if !head.SharePose(eyes) || len(head.Parts) != 3 {
		t.Fatalf("parts = %+v", head.Parts)
	}

	shared := head.Parts[2]

	if !shared.SharesPose {
		t.Error("the shared part is not posed by the head's bones")
	}

	if shared.Placement.Offset != placed.Offset || shared.Attachment != 2 || len(shared.Additive) != 1 || shared.Additive[0].ID != "slide" {
		t.Errorf("shared part = %+v", shared)
	}

	if len(shared.Attributes) != 2 || shared.Attributes[0].Name != "slide_far" || shared.Attributes[1].Name != "big_eyes" {
		t.Errorf("attributes = %+v", shared.Attributes)
	}

	// Raised with the head: the quad of two units now from 9 to 11.
	if shared.Pieces[0].Positions[1] != 9 || head.Max[1] != 11 {
		t.Errorf("first corner at %v, bounds to %v", shared.Pieces[0].Positions[:3], head.Max)
	}

	if (&Model{}).SharePose(eyes) {
		t.Error("shared the pose of a model with no skeleton")
	}
}

// The bones of hair rigged with a namespace are named as the head names them,
// which is how everything that poses them finds them; a bone the head does
// not have keeps its name.
func TestSharePoseRenamesNamespacedBones(t *testing.T) {
	bone := func(name string) mesh.Bone {
		return mesh.Bone{Name: name, Parent: -1, Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}}
	}

	head := &Model{Parts: []Part{{Name: "head", Pieces: Convert(quadMesh(t)), Skeleton: []mesh.Bone{bone("head_root"), bone("bn_h_head")}, Placement: mat.Identity()}}}

	rig := []mesh.Bone{bone("male_head_rig_01:head_root"), bone("male_head_rig_01:bn_h_head"), bone("male_head_rig_01:bn_hair_tail")}
	hair := &Model{Parts: []Part{{Name: "hair", Pieces: Convert(quadMesh(t)), Skeleton: rig, Placement: mat.Identity()}}}

	if !head.SharePose(hair) {
		t.Fatal("the hair did not share the pose of the head")
	}

	var names []string
	for _, bone := range head.Parts[1].Skeleton {
		names = append(names, bone.Name)
	}

	if want := []string{"head_root", "bn_h_head", "male_head_rig_01:bn_hair_tail"}; !slices.Equal(names, want) {
		t.Errorf("bones = %v, want %v", names, want)
	}

	if rig[0].Name != "male_head_rig_01:head_root" {
		t.Errorf("the hair's own skeleton was renamed to %s", rig[0].Name)
	}
}

// A model seated on another takes the other's normals where their vertices
// meet, and keeps its own everywhere else.
func TestMatchNormals(t *testing.T) {
	head := &Model{Parts: []Part{{Pieces: []Piece{{
		Positions: []float32{0, 0, 0, 5, 5, 5},
		Normals:   []float32{0, 1, 0, 0, 1, 0},
	}}}}}

	body := &Model{Parts: []Part{{Pieces: []Piece{{
		Positions: []float32{0.0004, 0, 0, 9, 9, 9},
		Normals:   []float32{1, 0, 0, 0, 0, 1},
	}}}}}

	if changed := head.MatchNormals(body, 0.001); changed != 1 {
		t.Errorf("changed %d vertices, want the one that meets the body", changed)
	}

	normals := head.Parts[0].Pieces[0].Normals
	if !slices.Equal(normals[:3], []float32{1, 0, 0}) || !slices.Equal(normals[3:], []float32{0, 1, 0}) {
		t.Errorf("normals = %v, want the body's where they meet and the head's own elsewhere", normals)
	}
}
