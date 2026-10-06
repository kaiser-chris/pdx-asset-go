package entity

import (
	"math"

	"github.com/kaiser-chris/pdx-parser-go/asset"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// placement puts an attached entity where it hangs, in the coordinates of the
// games' files, which are those of Direct3D: a point is a row, multiplied by
// the linear part and moved by the offset, p·linear + offset. Every matrix
// the mesh files store is laid out that way, a row per axis and the offset
// last.
type placement struct {
	linear [3][3]float64
	offset [3]float64
}

// identity leaves everything where it is.
var identity = placement{linear: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

// then is p followed by q: what p places, q places again, the way an
// attachment's own placement is followed by that of the entity it hangs from.
func (p placement) then(q placement) placement {
	var combined placement

	for row := range 3 {
		for column := range 3 {
			for k := range 3 {
				combined.linear[row][column] += p.linear[row][k] * q.linear[k][column]
			}
		}
	}

	combined.offset = q.point(p.offset)

	return combined
}

// point places a point.
func (p placement) point(v [3]float64) [3]float64 {
	moved := p.direction(v)

	for axis := range 3 {
		moved[axis] += p.offset[axis]
	}

	return moved
}

// direction turns a direction, which the offset does not move.
func (p placement) direction(v [3]float64) [3]float64 {
	var turned [3]float64

	for column := range 3 {
		for k := range 3 {
			turned[column] += v[k] * p.linear[k][column]
		}
	}

	return turned
}

func (p placement) determinant() float64 {
	m := p.linear

	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// inverse undoes a placement. One that flattens everything has none.
func (p placement) inverse() (placement, bool) {
	determinant := p.determinant()
	if math.Abs(determinant) < 1e-12 {
		return placement{}, false
	}

	m := p.linear

	var inverted placement

	for row := range 3 {
		for column := range 3 {
			// The cofactor of the transposed position, over the
			// determinant.
			a, b := (column+1)%3, (column+2)%3
			c, d := (row+1)%3, (row+2)%3
			inverted.linear[row][column] = (m[a][c]*m[b][d] - m[a][d]*m[b][c]) / determinant
		}
	}

	moved := inverted.direction(p.offset)
	inverted.offset = [3]float64{-moved[0], -moved[1], -moved[2]}

	return inverted, true
}

// normals is the placement normals are turned by: the inverse of the linear
// part, transposed, which keeps them at right angles to their surface where
// it is stretched more one way than another.
func (p placement) normals() placement {
	inverted, ok := p.inverse()
	if !ok {
		return identity
	}

	var transposed placement

	for row := range 3 {
		for column := range 3 {
			transposed.linear[row][column] = inverted.linear[column][row]
		}
	}

	return transposed
}

func (p placement) isIdentity() bool {
	return p == identity
}

// scaling resizes everything by a factor.
func scaling(factor float64) placement {
	return placement{linear: [3][3]float64{{factor, 0, 0}, {0, factor, 0}, {0, 0, factor}}}
}

// fromMatrix reads a matrix a mesh file stores: four rows of three, as a
// bone's, or of four, as a locator's, the axes first and the offset last.
func fromMatrix(values []float32) (placement, bool) {
	width := 0

	switch len(values) {
	case 12:
		width = 3
	case 16:
		width = 4
	default:
		return placement{}, false
	}

	var read placement

	for row := range 3 {
		for column := range 3 {
			read.linear[row][column] = float64(values[row*width+column])
		}
	}

	for axis := range 3 {
		read.offset[axis] = float64(values[3*width+axis])
	}

	return read, true
}

// fromQuaternion places by a rotation, a quaternion of x, y, z and w, and an
// offset. The mesh files write the rotation that does nothing as w = -1 as
// often as w = 1, which is the same rotation.
func fromQuaternion(rotation [4]float32, offset [3]float32) placement {
	x, y, z, w := float64(rotation[0]), float64(rotation[1]), float64(rotation[2]), float64(rotation[3])

	length := math.Sqrt(x*x + y*y + z*z + w*w)
	if length == 0 {
		x, y, z, w, length = 0, 0, 0, 1, 1
	}

	x, y, z, w = x/length, y/length, z/length, w/length

	// The matrix of the rotation for columns, transposed for rows.
	return placement{
		linear: [3][3]float64{
			{1 - 2*(y*y+z*z), 2 * (x*y + w*z), 2 * (x*z - w*y)},
			{2 * (x*y - w*z), 1 - 2*(x*x+z*z), 2 * (y*z + w*x)},
			{2 * (x*z + w*y), 2 * (y*z - w*x), 1 - 2*(x*x+y*y)},
		},
		offset: [3]float64{float64(offset[0]), float64(offset[1]), float64(offset[2])},
	}
}

// fromEntityLocator places by a locator of an asset file: its scale, then its
// rotation in degrees around x, y and z in that order, then its position.
// The shipped files rotate few of their locators, and those around one axis,
// so the order the engine turns them in is not known from them.
func fromEntityLocator(locator asset.Locator) placement {
	placed := scaling(locator.Scale)

	for axis, degrees := range []float64{locator.Rotation.X, locator.Rotation.Y, locator.Rotation.Z} {
		if degrees != 0 {
			placed = placed.then(turning(axis, degrees*math.Pi/180))
		}
	}

	placed.offset = [3]float64{locator.Position.X, locator.Position.Y, locator.Position.Z}

	return placed
}

// turning is a rotation around one axis, by an angle in radians, the way
// Direct3D turns: clockwise looking along the axis towards the origin.
func turning(axis int, angle float64) placement {
	sin, cos := math.Sincos(angle)
	turned := identity

	a, b := (axis+1)%3, (axis+2)%3
	turned.linear[a][a], turned.linear[a][b] = cos, sin
	turned.linear[b][a], turned.linear[b][b] = -sin, cos

	return turned
}

// fromMeshLocator places by a locator of a mesh file: by the matrix it
// stores, which holds its scale as well, or failing that by its rotation and
// position.
func fromMeshLocator(locator mesh.Locator) placement {
	if placed, ok := fromMatrix(locator.Transform); ok {
		return placed
	}

	return fromQuaternion(locator.Rotation, locator.Position)
}

// fromBone places at a bone in the pose the mesh is stored in: the inverse of
// the matrix the file stores, which undoes that pose.
func fromBone(bone mesh.Bone) (placement, bool) {
	undoing, ok := fromMatrix(bone.Transform)
	if !ok {
		return placement{}, false
	}

	return undoing.inverse()
}

// placeMesh returns a copy of a mesh with its vertices placed, in the
// coordinates of the file, before it is converted. A placement that mirrors
// winds the triangles the other way round, so that their front stays in
// front.
func placeMesh(source *mesh.Mesh, placed placement) *mesh.Mesh {
	if placed.isIdentity() {
		return source
	}

	copied := *source
	copied.Positions = make([]float32, len(source.Positions))
	copied.Normals = make([]float32, len(source.Normals))
	copied.Tangents = make([]float32, len(source.Tangents))

	for vertex := 0; vertex+2 < len(source.Positions); vertex += 3 {
		moved := placed.point(widen(source.Positions[vertex:]))
		copy(copied.Positions[vertex:], narrow(moved))
	}

	turning := placed.normals()

	for vertex := 0; vertex+2 < len(source.Normals); vertex += 3 {
		copy(copied.Normals[vertex:], narrow(unit(turning.direction(widen(source.Normals[vertex:])))))
	}

	for vertex := 0; vertex+3 < len(source.Tangents); vertex += 4 {
		copy(copied.Tangents[vertex:], narrow(unit(placed.direction(widen(source.Tangents[vertex:])))))
		copied.Tangents[vertex+3] = source.Tangents[vertex+3]
	}

	if placed.determinant() < 0 {
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
