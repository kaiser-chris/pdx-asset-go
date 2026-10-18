package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// A model can also hang from a single joint of another, rigidly, as a portrait
// accessory with a node does: a sabre in the hand, a pipe between the lips.
//
//	prop_saber = {
//		entity = { required_tags = "" node = "bn_l_prop" entity = "prop_saber_entity" }
//	}
//
// The mesh of such a model has no skeleton. It is made in the space of the
// joint it hangs from, and is drawn wherever the other model's poses take
// that joint, turned and scaled with it.

// Hang hangs the model from a joint of another, which it is drawn at from now
// on wherever the other's poses take the joint. A nil parent lets go of it
// again, and the model is drawn where its mesh puts it.
//
// The model is drawn at the joint as the other was last posed, so the other
// has to be posed before the model is drawn. A joint the other does not have
// leaves the model where its mesh puts it.
func (m *Model) Hang(parent *Model, joint string) {
	m.hangsFrom = parent
	m.hangJoint = joint
}

// Hanging reports whether the model hangs from a joint of another that it can
// be drawn at.
func (m *Model) Hanging() bool {
	_, ok := m.hungAt()

	return ok
}

// placed is the transform the model is drawn with: the one it is drawn by,
// after the joint it hangs from when it hangs from one.
func (m *Model) placed(transform rl.Matrix) rl.Matrix {
	hung, ok := m.hungAt()
	if !ok {
		return transform
	}

	return rl.MatrixMultiply(hung, transform)
}

// hungAt is where the joint the model hangs from is now, in the coordinates
// models are drawn in.
func (m *Model) hungAt() (rl.Matrix, bool) {
	if m.hangsFrom == nil {
		return rl.Matrix{}, false
	}

	_, posed, ok := m.hangsFrom.joint(m.hangJoint)
	if !ok {
		return rl.Matrix{}, false
	}

	// The joint is in the coordinates of the files, and the model's vertices
	// in those mirrored across x: they are mirrored back, placed by the
	// joint, and mirrored again.
	return matrixOf(model.Mirror.Then(posed).Then(model.Mirror)), true
}

// matrixOf is a transform as raylib takes one. A transform places a point
// written as a row, by its rows; raylib places a column, by its columns, so
// the linear part is transposed and the offset is the last column.
func matrixOf(t mat.Transform) rl.Matrix {
	l, o := t.Linear, t.Offset

	return rl.Matrix{
		M0: float32(l[0][0]), M4: float32(l[1][0]), M8: float32(l[2][0]), M12: float32(o[0]),
		M1: float32(l[0][1]), M5: float32(l[1][1]), M9: float32(l[2][1]), M13: float32(o[1]),
		M2: float32(l[0][2]), M6: float32(l[1][2]), M10: float32(l[2][2]), M14: float32(o[2]),
		M15: 1,
	}
}
