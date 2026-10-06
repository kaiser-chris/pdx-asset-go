package entity

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"

	"github.com/kaiser-chris/pdx-asset-go/mat"
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

// A bone stores the inverse of where it is; a locator of an asset file is
// turned in degrees, the way Direct3D turns.
func TestBonesAndLocatorsOfAssetFiles(t *testing.T) {
	bone := mesh.Bone{Name: "spine", Transform: []float32{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, -5, 0}}

	placed, ok := fromBone(bone)
	if !ok || !near(placed.Point([3]float64{}), [3]float64{0, 5, 0}) {
		t.Errorf("bone at %v, %v; want 5 up", placed.Offset, ok)
	}

	if _, ok := fromBone(mesh.Bone{Name: "unplaced"}); ok {
		t.Error("a bone without a transform was placed")
	}

	locator := asset.Locator{Position: asset.Vec3{X: 1, Y: 2, Z: 3}, Rotation: asset.Vec3{Y: 90}, Scale: 2}

	placed = fromEntityLocator(locator)

	// Along x, doubled and turned a quarter around y, ends along -z.
	if got := placed.Point([3]float64{1, 0, 0}); !near(got, [3]float64{1, 2, 1}) {
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

	mirrored := mat.Scaling(3)
	mirrored.Linear[0][0] = -3

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
