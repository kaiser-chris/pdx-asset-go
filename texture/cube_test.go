package texture

import (
	"math"
	"testing"
)

// Looking a direction up on a face and turning the face's texel back into a
// direction gives the same direction, on every face.
func TestCubeFaceRoundTrip(t *testing.T) {
	for face := range 6 {
		for _, uv := range [][2]float64{{0.5, 0.5}, {0.2, 0.7}, {0.9, 0.1}} {
			direction := CubeDirection(face, uv[0], uv[1])
			gotFace, u, v := cubeFace(direction)

			if gotFace != face || math.Abs(u-uv[0]) > 1e-9 || math.Abs(v-uv[1]) > 1e-9 {
				t.Errorf("face %d at %v: direction %v comes back as face %d at %v %v", face, uv, direction, gotFace, u, v)
			}
		}
	}
}

// The middle of the picture looks along -z, a quarter of the way across along
// -x, three quarters along +x, its edges along +z, and its top straight up.
func TestEquirectangularDirection(t *testing.T) {
	for _, test := range []struct {
		u, v float64
		want [3]float64
	}{
		{0.5, 0.5, [3]float64{0, 0, -1}},
		{0.75, 0.5, [3]float64{1, 0, 0}},
		{0.25, 0.5, [3]float64{-1, 0, 0}},
		{0, 0.5, [3]float64{0, 0, 1}},
		{0.5, 0, [3]float64{0, 1, 0}},
	} {
		got := EquirectangularDirection(test.u, test.v)
		for axis := range 3 {
			if math.Abs(got[axis]-test.want[axis]) > 1e-9 {
				t.Errorf("u %v v %v looks along %v, want %v", test.u, test.v, got, test.want)

				break
			}
		}
	}
}

// Every face of a cubemap of six colours lands where its axis is in the
// picture laid out from it.
func TestEquirectangular(t *testing.T) {
	colours := [6][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}, {0, 255, 255, 255}, {255, 0, 255, 255}}

	var faces [6]*Image

	for index, colour := range colours {
		faces[index] = &Image{Width: 1, Height: 1, Format: RGBA8, Levels: 1, Data: colour[:]}
	}

	pixels, err := Equirectangular(faces, 64, 32)
	if err != nil {
		t.Fatal(err)
	}

	at := func(u, v float64) [4]byte {
		offset := pixels.At(int(u*64), int(v*32))

		return [4]byte(pixels.Data[offset : offset+4])
	}

	for _, test := range []struct {
		u, v float64
		face int
	}{
		{0.75, 0.5, 0}, // +x
		{0.25, 0.5, 1}, // -x
		{0.5, 0.01, 2}, // +y
		{0.5, 0.99, 3}, // -y
		{0.01, 0.5, 4}, // +z
		{0.5, 0.5, 5},  // -z
	} {
		if got := at(test.u, test.v); got != colours[test.face] {
			t.Errorf("u %v v %v = %v, want face %d's %v", test.u, test.v, got, test.face, colours[test.face])
		}
	}

	if _, err := Equirectangular([6]*Image{}, 8, 4); err == nil {
		t.Error("laid out a cubemap of no faces")
	}
}
