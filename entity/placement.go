package entity

import (
	"math"

	"github.com/kaiser-chris/pdx-parser-go/asset"

	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// placement puts an attached entity where it hangs, in the coordinates of the
// games' files, which are those of Direct3D. The arithmetic is in the mat
// package; this file interprets the games' locators and bones into it.
type placement = mat.Transform

// identity leaves everything where it is.
var identity = mat.Identity()

// fromEntityLocator places by a locator of an asset file: its scale, then its
// rotation in degrees around x, y and z in that order, then its position.
// The shipped files rotate few of their locators, and those around one axis,
// so the order the engine turns them in is not known from them.
func fromEntityLocator(locator asset.Locator) placement {
	placed := mat.Scaling(locator.Scale)

	for axis, degrees := range []float64{locator.Rotation.X, locator.Rotation.Y, locator.Rotation.Z} {
		if degrees != 0 {
			placed = placed.Then(mat.Turning(axis, degrees*math.Pi/180))
		}
	}

	placed.Offset = [3]float64{locator.Position.X, locator.Position.Y, locator.Position.Z}

	return placed
}

// fromMeshLocator places by a locator of a mesh file: by the matrix it
// stores, which holds its scale as well, or failing that by its rotation and
// position.
func fromMeshLocator(locator mesh.Locator) placement {
	if placed, ok := mat.FromMatrix(locator.Transform); ok {
		return placed
	}

	return mat.FromQuaternion(locator.Rotation, locator.Position)
}

// fromBone places at a bone in the pose the mesh is stored in: the inverse of
// the matrix the file stores, which undoes that pose.
func fromBone(bone mesh.Bone) (placement, bool) {
	undoing, ok := mat.FromMatrix(bone.Transform)
	if !ok {
		return placement{}, false
	}

	return undoing.Inverse()
}

// placeMesh returns a copy of a mesh with its vertices placed, in the
// coordinates of the file, before it is converted. A placement that mirrors
// winds the triangles the other way round, so that their front stays in
// front.
func placeMesh(source *mesh.Mesh, placed placement) *mesh.Mesh {
	if placed.IsIdentity() {
		return source
	}

	copied := *source
	copied.Positions = make([]float32, len(source.Positions))
	copied.Normals = make([]float32, len(source.Normals))
	copied.Tangents = make([]float32, len(source.Tangents))

	for vertex := 0; vertex+2 < len(source.Positions); vertex += 3 {
		moved := placed.Point(widen(source.Positions[vertex:]))
		copy(copied.Positions[vertex:], narrow(moved))
	}

	normals := placed.Normals()

	for vertex := 0; vertex+2 < len(source.Normals); vertex += 3 {
		copy(copied.Normals[vertex:], narrow(unit(normals.Direction(widen(source.Normals[vertex:])))))
	}

	for vertex := 0; vertex+3 < len(source.Tangents); vertex += 4 {
		copy(copied.Tangents[vertex:], narrow(unit(placed.Direction(widen(source.Tangents[vertex:])))))
		copied.Tangents[vertex+3] = source.Tangents[vertex+3]
	}

	if placed.Determinant() < 0 {
		copied.Indices = make([]uint32, len(source.Indices))
		for triangle := 0; triangle+2 < len(source.Indices); triangle += 3 {
			copied.Indices[triangle] = source.Indices[triangle]
			copied.Indices[triangle+1] = source.Indices[triangle+2]
			copied.Indices[triangle+2] = source.Indices[triangle+1]
		}

		// The bitangent follows the mirror as well.
		for vertex := 3; vertex < len(copied.Tangents); vertex += 4 {
			copied.Tangents[vertex] = -copied.Tangents[vertex]
		}
	}

	return &copied
}

func widen(v []float32) [3]float64 {
	return [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
}

func narrow(v [3]float64) []float32 {
	return []float32{float32(v[0]), float32(v[1]), float32(v[2])}
}

func unit(v [3]float64) [3]float64 {
	length := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if length == 0 {
		return v
	}

	return [3]float64{v[0] / length, v[1] / length, v[2] / length}
}
