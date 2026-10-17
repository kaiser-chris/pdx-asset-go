package texture

import (
	"fmt"
	"math"
)

// A cubemap is six pictures of what surrounds a point, one along each axis.
// A graphics card looks one up by a direction; this package can lay one out
// as a single picture instead, of longitude across and latitude down, which a
// renderer that can only bind flat textures looks up by the same direction.

// CubeDirection is the direction a texel of a cubemap face looks along, for a
// face in the order DecodeCubeDDS gives them, +x, -x, +y, -y, +z, -z, at u and
// v from 0 to 1 across it, as Direct3D and OpenGL lay the faces out alike.
func CubeDirection(face int, u, v float64) [3]float64 {
	s, t := 2*u-1, 2*v-1

	var direction [3]float64

	switch face {
	case 0:
		direction = [3]float64{1, -t, -s}
	case 1:
		direction = [3]float64{-1, -t, s}
	case 2:
		direction = [3]float64{s, 1, t}
	case 3:
		direction = [3]float64{s, -1, -t}
	case 4:
		direction = [3]float64{s, -t, 1}
	default:
		direction = [3]float64{-s, -t, -1}
	}

	return unitVector(direction)
}

// cubeFace is the face of a cubemap a direction looks into, and u and v from
// 0 to 1 across it; the inverse of CubeDirection.
func cubeFace(direction [3]float64) (face int, u, v float64) {
	x, y, z := direction[0], direction[1], direction[2]
	ax, ay, az := math.Abs(x), math.Abs(y), math.Abs(z)

	var s, t, major float64

	switch {
	case ax >= ay && ax >= az && x > 0:
		face, s, t, major = 0, -z, -y, ax
	case ax >= ay && ax >= az:
		face, s, t, major = 1, z, -y, ax
	case ay >= az && y > 0:
		face, s, t, major = 2, x, z, ay
	case ay >= az:
		face, s, t, major = 3, x, -z, ay
	case z > 0:
		face, s, t, major = 4, x, -y, az
	default:
		face, s, t, major = 5, -x, -y, az
	}

	if major == 0 {
		return 4, 0.5, 0.5
	}

	return face, (s/major + 1) / 2, (t/major + 1) / 2
}

// EquirectangularDirection is the direction a texel of a picture laid out by
// Equirectangular looks along, at u and v from 0 to 1 across and down it: u
// goes once round, from -z through +x, and v from straight up to straight
// down.
func EquirectangularDirection(u, v float64) [3]float64 {
	longitude := (u - 0.5) * 2 * math.Pi
	latitude := v * math.Pi

	return [3]float64{math.Sin(latitude) * math.Sin(longitude), math.Cos(latitude), -math.Sin(latitude) * math.Cos(longitude)}
}

// Equirectangular lays the faces of a cubemap out as a single picture of a
// width and a height, of longitude across and latitude down, as
// EquirectangularDirection says, read from the largest level of each face, the
// nearest texel. A face whose pixels cannot be read leaves its part black.
func Equirectangular(faces [6]*Image, width, height int) (*Pixels, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("a picture of %dx%d cannot be laid out", width, height)
	}

	for index, face := range faces {
		if face == nil || face.Width <= 0 || face.Height <= 0 {
			return nil, fmt.Errorf("face %d of the cubemap is missing", index+1)
		}
	}

	pixels := NewPixels(width, height)

	for y := range height {
		for x := range width {
			direction := EquirectangularDirection((float64(x)+0.5)/float64(width), (float64(y)+0.5)/float64(height))
			face, u, v := cubeFace(direction)

			image := faces[face]
			column := min(int(u*float64(image.Width)), image.Width-1)
			row := min(int(v*float64(image.Height)), image.Height-1)

			colour, _ := image.Colour(column, row)
			copy(pixels.Data[pixels.At(x, y):], colour[:])
			pixels.Data[pixels.At(x, y)+3] = 255
		}
	}

	return pixels, nil
}

// unitVector scales a direction to unit length.
func unitVector(direction [3]float64) [3]float64 {
	length := math.Sqrt(direction[0]*direction[0] + direction[1]*direction[1] + direction[2]*direction[2])
	if length == 0 {
		return direction
	}

	return [3]float64{direction[0] / length, direction[1] / length, direction[2] / length}
}
