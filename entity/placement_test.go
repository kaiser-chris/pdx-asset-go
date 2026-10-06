package entity

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

func near(a, b [3]float64) bool {
	for axis := range 3 {
		if math.Abs(a[axis]-b[axis]) > 1e-5 {
			return false
		}
	}

	return true
}

// A locator of a mesh file stores its rotation twice, as a quaternion and in
// its matrix, which have to agree. The numbers are those of an oar's locator,
// turned a few degrees around z, with its rotation written with w below
// zero, as the files do.
func TestQuaternionAgreesWithTheMatrix(t *testing.T) {
	position := [3]float32{-0.13067758, 0.30782664, -1.2628052}

	stored, ok := fromMatrix([]float32{
		0.99785846, -0.06540942, -0.00023805407, 0,
		0.06540879, 0.9978564, -0.0020706223, 0,
		0.00037298197, 0.0020506172, 0.99999785, 0,
		position[0], position[1], position[2], 1,
	})
	if !ok {
		t.Fatal("a matrix of sixteen was not read")
	}

	turned := fromQuaternion([4]float32{0.0010308624, -0.00015284095, 0.032722093, -0.9994639}, position)

	for row := range 3 {
		if !near(turned.linear[row], stored.linear[row]) {
			t.Errorf("row %d = %v, want the stored %v", row, turned.linear[row], stored.linear[row])
		}
	}

	if turned.offset != stored.offset {
		t.Errorf("offset = %v, want %v", turned.offset, stored.offset)
	}

	// The rotation that does nothing, written either way round.
	for _, w := range []float32{1, -1} {
		if still := fromQuaternion([4]float32{0, 0, 0, w}, [3]float32{}); !still.isIdentity() {
			t.Errorf("w = %v turns: %+v", w, still)
		}
	}
}

// The matrices are rows, the offset last, applied to the point on their
// left: a point is placed by what comes first, then by what follows.
func TestPlacementsCompose(t *testing.T) {
	moved := identity
	moved.offset = [3]float64{1, 0, 0}

	quarter := turning(1, math.Pi/2)

	// Moved along x, then turned a quarter around y, which takes x to -z.
	if got := moved.then(quarter).point([3]float64{}); !near(got, [3]float64{0, 0, -1}) {
		t.Errorf("moved, then turned: %v", got)
	}

	// Turned first, the origin stays put, then moves along x.
	if got := quarter.then(moved).point([3]float64{}); !near(got, [3]float64{1, 0, 0}) {
		t.Errorf("turned, then moved: %v", got)
	}

	whole := scaling(2).then(quarter).then(moved)

	undone, ok := whole.inverse()
	if !ok {
		t.Fatal("no inverse")
	}

	point := [3]float64{3, -4, 5}
	if got := undone.point(whole.point(point)); !near(got, point) {
		t.Errorf("placed and undone: %v, want %v", got, point)
	}

	if _, ok := scaling(0).inverse(); ok {
		t.Error("a placement that flattens everything was undone")
	}
}

// A bone stores the inverse of where it is; a locator of an asset file is
// turned in degrees, the way Direct3D turns.
func TestBonesAndLocatorsOfAssetFiles(t *testing.T) {
	bone := mesh.Bone{Name: "spine", Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, -5, 0}}

	placed, ok := fromBone(bone)
	if !ok || !near(placed.point([3]float64{}), [3]float64{0, 5, 0}) {
		t.Errorf("bone at %v, %v; want 5 up", placed.offset, ok)
	}

	if _, ok := fromBone(mesh.Bone{Name: "unplaced"}); ok {
		t.Error("a bone without a transform was placed")
	}

	locator := asset.Locator{Position: asset.Vec3{X: 1, Y: 2, Z: 3}, Rotation: asset.Vec3{Y: 90}, Scale: 2}

	placed = fromEntityLocator(locator)

	// Along x, doubled and turned a quarter around y, ends along -z.
	if got := placed.point([3]float64{1, 0, 0}); !near(got, [3]float64{1, 2, 1}) {
		t.Errorf("x placed at %v, want 2 back along z from the locator", got)
	}
}

// A placement that mirrors winds the triangles the other way round, and
// leaves every normal of unit length.
func TestPlaceMeshMirrored(t *testing.T) {
	quad := meshtest.Quad(2, 2)

	source := &mesh.Mesh{Positions: quad.Positions, Normals: quad.Normals, Tangents: quad.Tangents}
	for _, index := range quad.Indices {
		source.Indices = append(source.Indices, uint32(index))
	}

	if placeMesh(source, identity) != source {
		t.Error("a mesh left in place was copied")
	}

	mirrored := scaling(3)
	mirrored.linear[0][0] = -3

	placed := placeMesh(source, mirrored)

	if placed.Positions[3] != -3 || source.Positions[3] != 1 {
		t.Errorf("x of the second corner = %v, from %v", placed.Positions[3], source.Positions[3])
	}

	if placed.Indices[1] != source.Indices[2] || placed.Indices[2] != source.Indices[1] {
		t.Errorf("indices %v, want %v wound the other way", placed.Indices, source.Indices)
	}

	if placed.Normals[2] != -1 || placed.Tangents[0] != -1 || placed.Tangents[3] != -1 {
		t.Errorf("normal %v and tangent %v", placed.Normals[:3], placed.Tangents[:4])
	}
}
