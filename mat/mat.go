// Package mat holds the affine transforms of the games' files: a row vector
// multiplied by a linear part and moved by an offset, p·linear + offset, the
// way Direct3D writes a matrix as a row per axis with the offset last.
//
// It is the arithmetic that placing an attachment, positioning a bone and
// skinning a mesh all share, kept in plain Go so that everything that uses it
// can be tested without a GPU. The games' coordinates are left handed and the
// matrices are rows; turning them into what a graphics library wants is the
// model package's job, not this one's.
package mat

import "math"

// Transform places a point by a linear part and an offset.
type Transform struct {
	// Linear is the three by three part that turns and scales, a row per
	// axis.
	Linear [3][3]float64

	// Offset is the translation, the fourth row of the matrix the files
	// store.
	Offset [3]float64
}

// Identity is a transform that leaves everything where it is. It is a
// function so that every caller gets their own copy.
func Identity() Transform {
	return Transform{Linear: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
}

// Then is p followed by q: what p places, q places again, the way an
// attachment's own placement is followed by that of the entity it hangs from.
func (p Transform) Then(q Transform) Transform {
	var combined Transform

	for row := range 3 {
		for column := range 3 {
			for k := range 3 {
				combined.Linear[row][column] += p.Linear[row][k] * q.Linear[k][column]
			}
		}
	}

	combined.Offset = q.Point(p.Offset)

	return combined
}

// Point places a point.
func (p Transform) Point(v [3]float64) [3]float64 {
	moved := p.Direction(v)

	for axis := range 3 {
		moved[axis] += p.Offset[axis]
	}

	return moved
}

// Direction turns a direction, which the offset does not move.
func (p Transform) Direction(v [3]float64) [3]float64 {
	var turned [3]float64

	for column := range 3 {
		for k := range 3 {
			turned[column] += v[k] * p.Linear[k][column]
		}
	}

	return turned
}

// Determinant is the determinant of the linear part: the volume a transform
// turns a unit cube into, and negative for one that mirrors.
func (p Transform) Determinant() float64 {
	m := p.Linear

	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// Inverse undoes a transform. One that flattens everything has none.
func (p Transform) Inverse() (Transform, bool) {
	determinant := p.Determinant()
	if math.Abs(determinant) < 1e-12 {
		return Transform{}, false
	}

	m := p.Linear

	var inverted Transform

	for row := range 3 {
		for column := range 3 {
			// The cofactor of the transposed position, over the
			// determinant.
			a, b := (column+1)%3, (column+2)%3
			c, d := (row+1)%3, (row+2)%3
			inverted.Linear[row][column] = (m[a][c]*m[b][d] - m[a][d]*m[b][c]) / determinant
		}
	}

	moved := inverted.Direction(p.Offset)
	inverted.Offset = [3]float64{-moved[0], -moved[1], -moved[2]}

	return inverted, true
}

// Normals is the transform normals are turned by: the inverse of the linear
// part, transposed, which keeps them at right angles to their surface where
// it is stretched more one way than another.
func (p Transform) Normals() Transform {
	inverted, ok := p.Inverse()
	if !ok {
		return Identity()
	}

	var transposed Transform

	for row := range 3 {
		for column := range 3 {
			transposed.Linear[row][column] = inverted.Linear[column][row]
		}
	}

	return transposed
}

// IsIdentity reports whether the transform leaves everything where it is.
func (p Transform) IsIdentity() bool {
	return p == Identity()
}

// Scaling resizes everything by one factor.
func Scaling(factor float64) Transform {
	return Transform{Linear: [3][3]float64{{factor, 0, 0}, {0, factor, 0}, {0, 0, factor}}}
}

// Scale resizes each axis by its own factor, as a joint of an animation may
// scale one axis more than another.
func Scale(s [3]float64) Transform {
	return Transform{Linear: [3][3]float64{{s[0], 0, 0}, {0, s[1], 0}, {0, 0, s[2]}}}
}

// FromMatrix reads a matrix a mesh file stores: four rows of three, as a
// bone's, or of four, as a locator's, the axes first and the offset last.
func FromMatrix(values []float32) (Transform, bool) {
	width := 0

	switch len(values) {
	case 12:
		width = 3
	case 16:
		width = 4
	default:
		return Transform{}, false
	}

	var read Transform

	for row := range 3 {
		for column := range 3 {
			read.Linear[row][column] = float64(values[row*width+column])
		}
	}

	for axis := range 3 {
		read.Offset[axis] = float64(values[3*width+axis])
	}

	return read, true
}

// FromQuaternion places by a rotation, a quaternion of x, y, z and w, and an
// offset. The mesh files write the rotation that does nothing as w = -1 as
// often as w = 1, which is the same rotation.
func FromQuaternion(rotation [4]float32, offset [3]float32) Transform {
	x, y, z, w := float64(rotation[0]), float64(rotation[1]), float64(rotation[2]), float64(rotation[3])

	length := math.Sqrt(x*x + y*y + z*z + w*w)
	if length == 0 {
		x, y, z, w, length = 0, 0, 0, 1, 1
	}

	x, y, z, w = x/length, y/length, z/length, w/length

	// The matrix of the rotation for columns, transposed for rows.
	return Transform{
		Linear: [3][3]float64{
			{1 - 2*(y*y+z*z), 2 * (x*y + w*z), 2 * (x*z - w*y)},
			{2 * (x*y - w*z), 1 - 2*(x*x+z*z), 2 * (y*z + w*x)},
			{2 * (x*z + w*y), 2 * (y*z - w*x), 1 - 2*(x*x+y*y)},
		},
		Offset: [3]float64{float64(offset[0]), float64(offset[1]), float64(offset[2])},
	}
}

// Turning is a rotation around one axis, by an angle in radians, the way
// Direct3D turns: clockwise looking along the axis towards the origin.
func Turning(axis int, angle float64) Transform {
	sin, cos := math.Sincos(angle)
	turned := Identity()

	a, b := (axis+1)%3, (axis+2)%3
	turned.Linear[a][a], turned.Linear[a][b] = cos, sin
	turned.Linear[b][a], turned.Linear[b][b] = -sin, cos

	return turned
}
