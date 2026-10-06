package model

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// geometry reads a test mesh the way a real one is read.
func geometry(t *testing.T, writer *meshtest.Writer) *mesh.Mesh {
	t.Helper()

	file, err := mesh.Read(writer.Bytes())
	if err != nil || len(file.Shapes) == 0 || len(file.Shapes[0].Meshes) == 0 {
		t.Fatalf("read the test mesh: %+v, %v", file, err)
	}

	return &file.Shapes[0].Meshes[0]
}

func quadMesh(t *testing.T) *mesh.Mesh {
	t.Helper()

	return geometry(t, meshtest.New().Object(1, "object").Object(2, "quadShape").Mesh(meshtest.Quad(2, 2), ""))
}

func vec(values []float32, vertex, width int) [3]float32 {
	return [3]float32{values[vertex*width], values[vertex*width+1], values[vertex*width+2]}
}

func dot(a, b [3]float32) float32 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

// The games wind their triangles anticlockwise around the normal in right
// handed terms. Mirrored, they would be wound clockwise, which OpenGL takes
// for the back; converted, they are wound anticlockwise again.
func TestConvertKeepsTheFrontInFront(t *testing.T) {
	source := quadMesh(t)

	for triangle := 0; triangle < len(source.Indices); triangle += 3 {
		a, b, c := vec(source.Positions, int(source.Indices[triangle]), 3), vec(source.Positions, int(source.Indices[triangle+1]), 3), vec(source.Positions, int(source.Indices[triangle+2]), 3)
		if dot(cross(sub(b, a), sub(c, a)), vec(source.Normals, int(source.Indices[triangle]), 3)) <= 0 {
			t.Fatal("the test quad is not wound the way the games wind their triangles")
		}
	}

	pieces := Convert(source)
	if len(pieces) != 1 {
		t.Fatalf("%d pieces, want 1", len(pieces))
	}

	piece := pieces[0]

	for triangle := 0; triangle < len(piece.Indices); triangle += 3 {
		a, b, c := vec(piece.Positions, int(piece.Indices[triangle]), 3), vec(piece.Positions, int(piece.Indices[triangle+1]), 3), vec(piece.Positions, int(piece.Indices[triangle+2]), 3)
		normal := vec(piece.Normals, int(piece.Indices[triangle]), 3)

		if dot(cross(sub(b, a), sub(c, a)), normal) <= 0 {
			t.Errorf("triangle %d is wound clockwise around its normal, which OpenGL takes for the back", triangle/3)
		}
	}
}

// Mirroring across x turns positions and directions round, and keeps every
// tangent frame the frame it was, mirrored: the bitangent the shader works out
// is the mirror image of the one the game works out.
func TestConvertMirrorsAcrossX(t *testing.T) {
	source := quadMesh(t)

	// A tangent frame with some depth to it, so that the test means something.
	source.Normals = []float32{0.6, 0, -0.8, 0.6, 0, -0.8, 0.6, 0, -0.8, 0.6, 0, -0.8}
	source.Tangents = []float32{0.8, 0, 0.6, -1, 0.8, 0, 0.6, -1, 0.8, 0, 0.6, -1, 0.8, 0, 0.6, -1}

	piece := Convert(source)[0]

	for vertex := range source.Vertices() {
		position, converted := vec(source.Positions, vertex, 3), vec(piece.Positions, vertex, 3)
		if converted != [3]float32{-position[0], position[1], position[2]} {
			t.Errorf("vertex %d at %v, want %v mirrored", vertex, converted, position)
		}

		normal, tangent, sign := vec(source.Normals, vertex, 3), vec(source.Tangents, vertex, 4), source.Tangents[vertex*4+3]
		convertedNormal, convertedTangent, convertedSign := vec(piece.Normals, vertex, 3), vec(piece.Tangents, vertex, 4), piece.Tangents[vertex*4+3]

		if convertedNormal != [3]float32{-normal[0], normal[1], normal[2]} || convertedTangent != [3]float32{-tangent[0], tangent[1], tangent[2]} {
			t.Errorf("vertex %d: normal %v and tangent %v, want %v and %v mirrored", vertex, convertedNormal, convertedTangent, normal, tangent)
		}

		bitangent := scale(cross(normal, tangent), sign)
		convertedBitangent := scale(cross(convertedNormal, convertedTangent), convertedSign)

		if convertedBitangent != [3]float32{-bitangent[0], bitangent[1], bitangent[2]} {
			t.Errorf("vertex %d: bitangent %v, want %v mirrored", vertex, convertedBitangent, bitangent)
		}
	}

	// The texture coordinates are not touched: the pictures are the right way
	// round already, their first row at the top as OpenGL samples it.
	for index, value := range source.UV(0) {
		if piece.UV0[index] != value {
			t.Fatalf("texture coordinates %v, want %v", piece.UV0, source.UV(0))
		}
	}
}

func scale(vector [3]float32, factor float32) [3]float32 {
	return [3]float32{vector[0] * factor, vector[1] * factor, vector[2] * factor}
}

// The source mesh is not changed by converting it.
func TestConvertLeavesTheSourceAlone(t *testing.T) {
	source := quadMesh(t)
	before := append([]float32(nil), source.Positions...)
	indices := append([]uint32(nil), source.Indices...)

	Convert(source)

	for index := range before {
		if source.Positions[index] != before[index] {
			t.Fatal("converting changed the source positions")
		}
	}

	for index := range indices {
		if source.Indices[index] != indices[index] {
			t.Fatal("converting changed the source triangles")
		}
	}
}

// A mesh without normals, tangents or texture coordinates still converts to
// geometry with every attribute, the normals worked out from the triangles.
func TestConvertFillsInWhatIsMissing(t *testing.T) {
	quad := meshtest.Quad(2, 2)
	source := geometry(t, meshtest.New().Object(1, "object").Object(2, "s").
		Mesh(meshtest.Geometry{Positions: quad.Positions, Indices: quad.Indices}, ""))

	piece := Convert(source)[0]

	if len(piece.Normals) != 12 || len(piece.Tangents) != 16 || len(piece.UV0) != 8 || len(piece.UV1) != 8 {
		t.Fatalf("attributes: %d normal, %d tangent, %d and %d texture coordinate values",
			len(piece.Normals), len(piece.Tangents), len(piece.UV0), len(piece.UV1))
	}

	// The quad faces the camera of the game, along -z; mirroring across x
	// does not change that.
	for vertex := range 4 {
		if normal := vec(piece.Normals, vertex, 3); math.Abs(float64(normal[2]+1)) > 1e-6 {
			t.Errorf("worked out normal %v, want 0 0 -1", normal)
		}
	}
}

// Geometry of more vertices than sixteen bit indices reach is cut into pieces,
// each complete: every triangle is in exactly one piece, and every piece
// stays within its limit and names only vertices it has.
func TestConvertSplitsLargeMeshes(t *testing.T) {
	grid := meshtest.Grid(400, 300)
	source := geometry(t, meshtest.New().Object(1, "object").Object(2, "s").Mesh(grid, ""))

	pieces := Convert(source)
	if len(pieces) < 2 {
		t.Fatalf("%d pieces for %d vertices, want it split", len(pieces), source.Vertices())
	}

	triangles := 0

	for index, piece := range pieces {
		if piece.Vertices() > MaxPieceVertices || piece.Vertices() == 0 {
			t.Errorf("piece %d has %d vertices", index, piece.Vertices())
		}

		for name, values := range map[string]struct {
			values []float32
			width  int
		}{"normals": {piece.Normals, 3}, "tangents": {piece.Tangents, 4}, "uv0": {piece.UV0, 2}, "uv1": {piece.UV1, 2}} {
			if len(values.values) != piece.Vertices()*values.width {
				t.Errorf("piece %d: %d values of %s for %d vertices", index, len(values.values), name, piece.Vertices())
			}
		}

		for _, vertex := range piece.Indices {
			if int(vertex) >= piece.Vertices() {
				t.Fatalf("piece %d names vertex %d of %d", index, vertex, piece.Vertices())
			}
		}

		triangles += piece.Triangles()
	}

	if triangles != source.Triangles() {
		t.Errorf("%d triangles in the pieces, want the %d of the mesh", triangles, source.Triangles())
	}

	// The pieces together are the mesh, mirrored: the same area, and the same
	// corners of the box it fits in.
	model := Model{Parts: []Part{{Pieces: pieces}}}
	model.Bounds()

	low, high := source.Bounds()
	if model.Min != [3]float32{-high[0], low[1], low[2]} || model.Max != [3]float32{-low[0], high[1], high[2]} {
		t.Errorf("pieces fit in %v to %v, the mesh mirrored in %v to %v", model.Min, model.Max, low, high)
	}

	if area(pieces) != area(Convert(geometry(t, meshtest.New().Object(1, "object").Object(2, "s").Mesh(meshtest.Grid(400, 300), "")))) {
		t.Error("cutting the mesh into pieces changed its area")
	}
}

// area adds up the area of every triangle of the pieces, which is the same
// however the triangles are spread over pieces.
func area(pieces []Piece) float64 {
	total := 0.0

	for _, piece := range pieces {
		for triangle := 0; triangle < len(piece.Indices); triangle += 3 {
			a, b, c := vec(piece.Positions, int(piece.Indices[triangle]), 3), vec(piece.Positions, int(piece.Indices[triangle+1]), 3), vec(piece.Positions, int(piece.Indices[triangle+2]), 3)
			face := cross(sub(b, a), sub(c, a))
			total += math.Sqrt(float64(dot(face, face))) / 2
		}
	}

	return total
}

func TestBoundsOfNothing(t *testing.T) {
	model := Model{Min: [3]float32{1, 2, 3}}
	model.Bounds()

	if model.Min != [3]float32{} || model.Max != [3]float32{} {
		t.Errorf("an empty model fits in %v to %v", model.Min, model.Max)
	}
}

// The games keep two passes for decals; a part of either is a decal, a part
// of no pass or another is not.
func TestIsDecal(t *testing.T) {
	for subpass, want := range map[string]bool{"Decals": true, "LocalDecals": true, "": false, "Other": false} {
		if got := (Part{Subpass: subpass}).IsDecal(); got != want {
			t.Errorf("subpass %q is a decal: %v, want %v", subpass, got, want)
		}
	}
}

// The decals of the ground are drawn before those of a building, which lie
// over them, as Victoria 3's academy lays its garden over its cobbles.
func TestPass(t *testing.T) {
	for subpass, want := range map[string]int{"": 0, "Decals": 1, "LocalDecals": 2} {
		if got := (Part{Subpass: subpass}).Pass(); got != want {
			t.Errorf("subpass %q is drawn in pass %d, want %d", subpass, got, want)
		}
	}
}
