package mat

import (
	"math"
	"testing"
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

	stored, ok := FromMatrix([]float32{
		0.99785846, -0.06540942, -0.00023805407, 0,
		0.06540879, 0.9978564, -0.0020706223, 0,
		0.00037298197, 0.0020506172, 0.99999785, 0,
		position[0], position[1], position[2], 1,
	})
	if !ok {
		t.Fatal("a matrix of sixteen was not read")
	}

	turned := FromQuaternion([4]float32{0.0010308624, -0.00015284095, 0.032722093, -0.9994639}, position)

	for row := range 3 {
		if !near(turned.Linear[row], stored.Linear[row]) {
			t.Errorf("row %d = %v, want the stored %v", row, turned.Linear[row], stored.Linear[row])
		}
	}

	if turned.Offset != stored.Offset {
		t.Errorf("offset = %v, want %v", turned.Offset, stored.Offset)
	}

	// The rotation that does nothing, written either way round.
	for _, w := range []float32{1, -1} {
		if still := FromQuaternion([4]float32{0, 0, 0, w}, [3]float32{}); !still.IsIdentity() {
			t.Errorf("w = %v turns: %+v", w, still)
		}
	}
}

// The matrices are rows, the offset last, applied to the point on their
// left: a point is placed by what comes first, then by what follows.
func TestTransformsCompose(t *testing.T) {
	moved := Identity()
	moved.Offset = [3]float64{1, 0, 0}

	quarter := Turning(1, math.Pi/2)

	// Moved along x, then turned a quarter around y, which takes x to -z.
	if got := moved.Then(quarter).Point([3]float64{}); !near(got, [3]float64{0, 0, -1}) {
		t.Errorf("moved, then turned: %v", got)
	}

	// Turned first, the origin stays put, then moves along x.
	if got := quarter.Then(moved).Point([3]float64{}); !near(got, [3]float64{1, 0, 0}) {
		t.Errorf("turned, then moved: %v", got)
	}

	whole := Scaling(2).Then(quarter).Then(moved)

	undone, ok := whole.Inverse()
	if !ok {
		t.Fatal("no inverse")
	}

	point := [3]float64{3, -4, 5}
	if got := undone.Point(whole.Point(point)); !near(got, point) {
		t.Errorf("placed and undone: %v, want %v", got, point)
	}

	if _, ok := Scaling(0).Inverse(); ok {
		t.Error("a transform that flattens everything was undone")
	}
}

// Normals are turned by the inverse of the linear part, transposed, which
// keeps them at right angles to their surface when it is stretched more one
// way than another.
func TestNormalsTransform(t *testing.T) {
	// Squash y by two; a normal leaning into y is turned to lean twice as
	// far into it.
	stretched := Scale([3]float64{1, 0.5, 1})

	if got := stretched.Normals().Direction([3]float64{0, 1, 0}); !near(got, [3]float64{0, 2, 0}) {
		t.Errorf("a normal up is turned %v, want twice as long into y", got)
	}
}

// A per axis scale scales each axis by its own factor.
func TestScale(t *testing.T) {
	if got := Scale([3]float64{2, 3, 4}).Point([3]float64{1, 1, 1}); !near(got, [3]float64{2, 3, 4}) {
		t.Errorf("scaled to %v", got)
	}
}
