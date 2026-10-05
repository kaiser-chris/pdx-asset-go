// Package meshtest writes mesh files for tests.
//
// The meshes the games ship belong to them, so a test that needs one cannot
// use theirs. Writer builds a file record by record, as the games lay them
// out, including files that are broken on purpose; Quad and Model build the
// models the tests of this module draw.
package meshtest

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Writer builds a binary mesh file.
type Writer struct {
	file bytes.Buffer
}

// New starts a file, with the magic and the version every shipped file
// starts with.
func New() *Writer {
	writer := &Writer{}
	writer.file.WriteString("@@b@")
	writer.Ints("pdxasset", 1, 0)

	return writer
}

// Bytes returns the file written so far.
func (w *Writer) Bytes() []byte {
	return bytes.Clone(w.file.Bytes())
}

// Raw appends bytes as they are, for a file that is broken on purpose.
func (w *Writer) Raw(data ...byte) *Writer {
	w.file.Write(data)

	return w
}

// Object starts an object at the given depth: 1 for the top of the file.
func (w *Writer) Object(depth int, name string) *Writer {
	for range depth {
		w.file.WriteByte('[')
	}

	w.file.WriteString(name)
	w.file.WriteByte(0)

	return w
}

// Floats writes a property holding numbers with a fraction.
func (w *Writer) Floats(name string, values ...float32) *Writer {
	w.property(name, 'f', len(values))

	for _, value := range values {
		_ = binary.Write(&w.file, binary.LittleEndian, math.Float32bits(value))
	}

	return w
}

// Ints writes a property holding whole numbers.
func (w *Writer) Ints(name string, values ...int32) *Writer {
	w.property(name, 'i', len(values))

	for _, value := range values {
		_ = binary.Write(&w.file, binary.LittleEndian, value)
	}

	return w
}

// Strings writes a property holding strings.
func (w *Writer) Strings(name string, values ...string) *Writer {
	w.property(name, 's', len(values))

	for _, value := range values {
		// The length counts the zero that ends the string.
		_ = binary.Write(&w.file, binary.LittleEndian, uint32(len(value)+1))
		w.file.WriteString(value)
		w.file.WriteByte(0)
	}

	return w
}

// Property writes the start of a property with any kind and count, and no
// values, for a file that is broken on purpose.
func (w *Writer) Property(name string, kind byte, count uint32) *Writer {
	w.file.WriteByte('!')
	w.file.WriteByte(byte(len(name)))
	w.file.WriteString(name)
	w.file.WriteByte(kind)
	_ = binary.Write(&w.file, binary.LittleEndian, count)

	return w
}

func (w *Writer) property(name string, kind byte, count int) {
	w.Property(name, kind, uint32(count))
}

// Geometry is the vertices and triangles of one mesh.
type Geometry struct {
	Positions, Normals, Tangents, UV0 []float32
	Indices                           []int32
}

// Mesh writes a mesh object at depth 3, below a shape, with its geometry, its
// bounding box and a material. properties write further properties of the
// mesh, which come before the objects inside it, as they do in the games'
// files.
func (w *Writer) Mesh(geometry Geometry, diffuse string, properties ...func(w *Writer)) *Writer {
	w.Object(3, "mesh")
	w.Floats("p", geometry.Positions...)

	if geometry.Normals != nil {
		w.Floats("n", geometry.Normals...)
	}

	if geometry.Tangents != nil {
		w.Floats("ta", geometry.Tangents...)
	}

	if geometry.UV0 != nil {
		w.Floats("u0", geometry.UV0...)
	}

	w.Ints("tri", geometry.Indices...)

	for _, property := range properties {
		property(w)
	}

	low, high := bounds(geometry.Positions)
	w.Object(4, "aabb").Floats("min", low[:]...).Floats("max", high[:]...)
	w.Object(4, "material").Strings("shader", "standard").Strings("diff", diffuse)

	return w
}

// Quad is a square of the given size in the plane z = 0, facing the camera
// of the games, which looks along z from below zero, with its texture
// coordinates running from corner to corner, the first row of the picture at
// the top. Its triangles are wound as the games wind theirs: anticlockwise
// around the normal, in right handed terms.
func Quad(width, height float32) Geometry {
	half := [2]float32{width / 2, height / 2}

	return Geometry{
		Positions: []float32{
			-half[0], -half[1], 0,
			half[0], -half[1], 0,
			-half[0], half[1], 0,
			half[0], half[1], 0,
		},
		Normals:  []float32{0, 0, -1, 0, 0, -1, 0, 0, -1, 0, 0, -1},
		Tangents: []float32{1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1},
		UV0:      []float32{0, 1, 1, 1, 0, 0, 1, 0},
		Indices:  []int32{0, 2, 1, 2, 3, 1},
	}
}

// QuadFile is a file holding one shape of one quad.
func QuadFile(width, height float32) []byte {
	return New().Object(1, "object").Object(2, "quadShape").Mesh(Quad(width, height), "quad_diffuse.dds").Bytes()
}

// Grid is a flat square of the given number of cells across and down, each
// cell two triangles, which is how a test gets a mesh with as many vertices as
// it wants.
func Grid(across, down int) Geometry {
	geometry := Geometry{}

	for y := range down + 1 {
		for x := range across + 1 {
			geometry.Positions = append(geometry.Positions, float32(x), float32(y), 0)
			geometry.Normals = append(geometry.Normals, 0, 0, -1)
			geometry.Tangents = append(geometry.Tangents, 1, 0, 0, 1)
			geometry.UV0 = append(geometry.UV0, float32(x)/float32(across), 1-float32(y)/float32(down))
		}
	}

	row := int32(across + 1)

	for y := range int32(down) {
		for x := range int32(across) {
			corner := y*row + x
			geometry.Indices = append(geometry.Indices,
				corner, corner+row, corner+1,
				corner+row, corner+row+1, corner+1)
		}
	}

	return geometry
}

func bounds(positions []float32) (low, high [3]float32) {
	if len(positions) < 3 {
		return low, high
	}

	low = [3]float32{positions[0], positions[1], positions[2]}
	high = low

	for index, value := range positions {
		low[index%3] = min(low[index%3], value)
		high[index%3] = max(high[index%3], value)
	}

	return low, high
}
