package mesh_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

func read(t *testing.T, data []byte) *mesh.File {
	t.Helper()

	file, err := mesh.Read(data)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	return file
}

func expectWarning(t *testing.T, file *mesh.File, fragment string) {
	t.Helper()

	for _, warning := range file.Warnings {
		if strings.Contains(warning, fragment) {
			return
		}
	}

	t.Errorf("no warning mentioning %q; got %q", fragment, file.Warnings)
}

func expectClean(t *testing.T, file *mesh.File) {
	t.Helper()

	if len(file.Warnings) > 0 {
		t.Errorf("unexpected warnings: %q", file.Warnings)
	}
}

func TestReadAQuad(t *testing.T) {
	file := read(t, meshtest.QuadFile(9, 6))
	expectClean(t, file)

	if !slices.Equal(file.Version, []int32{1, 0}) || len(file.Shapes) != 1 || file.Shapes[0].Name != "quadShape" {
		t.Fatalf("file = %+v", file)
	}

	shape := file.Shapes[0]
	if shape.LOD != 0 || len(shape.Meshes) != 1 || shape.Skeleton != nil {
		t.Fatalf("shape = %+v", shape)
	}

	quad := shape.Meshes[0]
	if quad.Vertices() != 4 || quad.Triangles() != 2 || len(quad.Normals) != 12 || len(quad.Tangents) != 16 {
		t.Errorf("%d vertices, %d triangles, %d normal and %d tangent values", quad.Vertices(), quad.Triangles(), len(quad.Normals), len(quad.Tangents))
	}

	if len(quad.UV(0)) != 8 || quad.UV(1) != nil || quad.UV(-1) != nil {
		t.Errorf("texture coordinates = %v", quad.UVs)
	}

	if !slices.Equal(quad.Indices, []uint32{0, 2, 1, 2, 3, 1}) {
		t.Errorf("indices = %v", quad.Indices)
	}

	if quad.Min != [3]float32{-4.5, -3, 0} || quad.Max != [3]float32{4.5, 3, 0} {
		t.Errorf("box = %v to %v", quad.Min, quad.Max)
	}

	if quad.Material != (mesh.Material{Shader: "standard", Diffuse: "quad_diffuse.dds"}) {
		t.Errorf("material = %+v", quad.Material)
	}

	low, high := quad.Bounds()
	if low != [3]float32{-4.5, -3, 0} || high != [3]float32{4.5, 3, 0} {
		t.Errorf("bounds = %v to %v", low, high)
	}
}

// A model as the portraits write theirs: two materials on one shape, a
// skeleton the meshes are skinned to, a level of detail, a third set of
// texture coordinates, properties nobody interprets, and locators.
func TestReadAWholeModel(t *testing.T) {
	quad := meshtest.Quad(2, 2)

	writer := meshtest.New().
		Object(1, "object").
		Floats("lodperc", 50, 10).
		Object(2, "bodyShape").
		Mesh(quad, "skin_diffuse.dds", func(w *meshtest.Writer) {
			w.Floats("u1", 0, 0, 0, 0, 0, 0, 0, 0).
				Floats("u2", 1, 1, 1, 1, 1, 1, 1, 1).
				Floats("boundingsphere", 0, 0, 0, 1.5).
				Floats("sfs", 1, 2)
		}).
		Object(4, "skin").
		Ints("bones", 2).
		Ints("ix", 0, -1, -1, -1, 0, 1, -1, -1, 1, -1, -1, -1, 1, 0, -1, -1).
		Floats("w", 1, 0, 0, 0, 0.5, 0.5, 0, 0, 1, 0, 0, 0, 0.75, 0.25, 0, 0).
		Mesh(quad, "cloth_diffuse.dds").
		Object(3, "skeleton").
		Object(4, "bn_root").Ints("ix", 0).Floats("tx", 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0).
		Object(4, "bn_spine").Ints("ix", 1).Ints("pa", 0).Floats("tx", 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, -1, 0).Ints("rcf", 3).
		Object(2, "bodyShape_lod1").Ints("lod", 1).Mesh(quad, "skin_diffuse.dds").
		Object(1, "locator").
		Object(2, "hand").Floats("p", 1, 2, 3).Floats("q", 0, 0, 0.7071, 0.7071).Strings("pa", "bn_spine").
		Object(2, "head").Floats("p", 0, 5, 0)

	file := read(t, writer.Bytes())
	expectClean(t, file)

	if !slices.Equal(file.LODPercentages, []float32{50, 10}) || len(file.Shapes) != 2 {
		t.Fatalf("file = %+v", file)
	}

	body := file.Shapes[0]
	if len(body.Meshes) != 2 || body.Meshes[1].Material.Diffuse != "cloth_diffuse.dds" {
		t.Fatalf("body meshes = %+v", body.Meshes)
	}

	first := body.Meshes[0]
	if len(first.UVs) != 3 || first.UV(2)[0] != 1 || first.Sphere != [4]float32{0, 0, 0, 1.5} {
		t.Errorf("mesh = %+v", first)
	}

	if len(first.Extra) != 1 || first.Extra[0].Name != "sfs" || !slices.Equal(first.Extra[0].Floats, []float32{1, 2}) {
		t.Errorf("extra properties = %+v, want sfs kept", first.Extra)
	}

	if first.Skin == nil || first.Skin.Influences != 2 || first.Skin.Joints[1] != -1 || first.Skin.Weights[13] != 0.25 {
		t.Errorf("skin = %+v", first.Skin)
	}

	if body.Meshes[1].Skin != nil {
		t.Error("the second mesh, which has no skin, has one")
	}

	if len(body.Skeleton) != 2 {
		t.Fatalf("skeleton = %+v", body.Skeleton)
	}

	root, spine := body.Skeleton[0], body.Skeleton[1]
	if root.Name != "bn_root" || root.Index != 0 || root.Parent != -1 || len(root.Transform) != 12 {
		t.Errorf("root = %+v", root)
	}

	if spine.Parent != 0 || spine.Transform[10] != -1 || len(spine.Extra) != 1 || spine.Extra[0].Name != "rcf" {
		t.Errorf("spine = %+v", spine)
	}

	if lod := file.Shapes[1]; lod.Name != "bodyShape_lod1" || lod.LOD != 1 {
		t.Errorf("level of detail = %+v", lod)
	}

	if len(file.Locators) != 2 {
		t.Fatalf("locators = %+v", file.Locators)
	}

	hand, head := file.Locators[0], file.Locators[1]
	if hand.Position != [3]float32{1, 2, 3} || hand.Rotation != [4]float32{0, 0, 0.7071, 0.7071} || hand.Parent != "bn_spine" {
		t.Errorf("hand = %+v", hand)
	}

	if head.Rotation != [4]float32{0, 0, 0, 1} || head.Parent != "" {
		t.Errorf("head = %+v, want no rotation and no parent", head)
	}

	// Only the most detailed shapes count towards the bounds.
	low, high, ok := file.Bounds()
	if !ok || low != [3]float32{-1, -1, 0} || high != [3]float32{1, 1, 0} {
		t.Errorf("bounds = %v to %v, %v", low, high, ok)
	}
}

// What does not add up is left out as narrowly as possible, and said.
func TestReadLeavesOutWhatDoesNotAddUp(t *testing.T) {
	quad := meshtest.Quad(2, 2)

	cases := []struct {
		name    string
		file    *meshtest.Writer
		warning string
		check   func(t *testing.T, file *mesh.File)
	}{
		{
			"positions that are no whole vertices",
			shape().Object(3, "mesh").Floats("p", 1, 2).Ints("tri", 0, 0, 0),
			"2 position values, want three per vertex; the mesh is left out",
			meshes(0),
		},
		{
			"no triangles",
			shape().Object(3, "mesh").Floats("p", quad.Positions...),
			"it has no triangles; the mesh is left out",
			meshes(0),
		},
		{
			"a triangle naming a vertex that does not exist",
			shape().Object(3, "mesh").Floats("p", quad.Positions...).Ints("tri", 0, 1, 4),
			"a triangle uses vertex 4 of 4; the mesh is left out",
			meshes(0),
		},
		{
			"a negative vertex",
			shape().Object(3, "mesh").Floats("p", quad.Positions...).Ints("tri", 0, -1, 2),
			"a triangle uses vertex -1 of 4",
			meshes(0),
		},
		{
			"vertex numbers that are no whole triangles",
			shape().Object(3, "mesh").Floats("p", quad.Positions...).Ints("tri", 0, 1, 2, 3),
			"4 vertex numbers, which is no whole number of triangles; the last 1 are left out",
			func(t *testing.T, file *mesh.File) {
				if mesh := file.Shapes[0].Meshes[0]; mesh.Triangles() != 1 {
					t.Errorf("triangles = %d, want the whole one", mesh.Triangles())
				}
			},
		},
		{
			"normals of the wrong count",
			shape().Object(3, "mesh").Floats("p", quad.Positions...).Floats("n", 0, 0, 1).Ints("tri", 0, 1, 2),
			"3 values of n for 4 vertices, want 3 each; left out",
			func(t *testing.T, file *mesh.File) {
				if mesh := file.Shapes[0].Meshes[0]; mesh.Normals != nil || mesh.Vertices() != 4 {
					t.Errorf("mesh = %+v, want it kept without normals", mesh)
				}
			},
		},
		{
			"texture coordinates of the wrong kind",
			shape().Object(3, "mesh").Floats("p", quad.Positions...).Ints("u0", 1, 2, 3, 4, 5, 6, 7, 8).Ints("tri", 0, 1, 2),
			"8 values of u0 for 4 vertices, want 2 each; left out",
			nil,
		},
		{
			"a skin of the wrong count",
			shape().Mesh(quad, "").Object(4, "skin").Ints("bones", 1).Ints("ix", 0, 0, 0, 0).Floats("w", 1, 1, 1, 1),
			"its skin has 4 bone indices and 4 weights for 4 vertices, want 4 each; the skin is left out",
			noSkin,
		},
		{
			"a skin of more bones than it stores",
			shape().Mesh(quad, "").Object(4, "skin").Ints("bones", 5).Ints("ix", skinOf(0, 0, 0, 0)...).Floats("w", weightsOf(4)...),
			"its skin says each vertex is moved by [5] bones, want one number from 1 to 4",
			noSkin,
		},
		{
			"a skin of no bones",
			shape().Mesh(quad, "").Object(4, "skin").Ints("bones", 0).Ints("ix").Floats("w"),
			"its skin says each vertex is moved by [0] bones",
			noSkin,
		},
		{
			"a skin naming a bone the skeleton does not have",
			shape().Mesh(quad, "").Object(4, "skin").Ints("bones", 1).Ints("ix", skinOf(0, 0, 0, 1)...).Floats("w", weightsOf(4)...).
				Object(3, "skeleton").Object(4, "only").Ints("ix", 0),
			"its skin names bone 1 of a skeleton of 1; the skin is left out",
			noSkin,
		},
		{
			"a bone hanging from one after it",
			shape().Mesh(quad, "").Object(3, "skeleton").Object(4, "first").Ints("ix", 0).Ints("pa", 1).Object(4, "second").Ints("ix", 1),
			"bone first: its parent [1] is not a bone before it; read as a root",
			func(t *testing.T, file *mesh.File) {
				if bone := file.Shapes[0].Skeleton[0]; bone.Parent != -1 {
					t.Errorf("bone = %+v, want a root", bone)
				}
			},
		},
		{
			"a bone whose index is not its place",
			shape().Mesh(quad, "").Object(3, "skeleton").Object(4, "first").Ints("ix", 5),
			"its index [5] is not its place 0 in the skeleton; read as 0",
			nil,
		},
		{
			"a bone transform of the wrong length",
			shape().Mesh(quad, "").Object(3, "skeleton").Object(4, "first").Ints("ix", 0).Floats("tx", 1, 2),
			"its transform is 2 values, want 12; ignored",
			nil,
		},
		{
			"a level of detail that is not a number",
			meshtest.New().Object(1, "object").Object(2, "s").Ints("lod", -1).Mesh(quad, ""),
			"its level of detail [-1] is not one number of at least 0; read as 0",
			nil,
		},
		{
			"a bounding sphere of the wrong length",
			shape().Mesh(quad, "", func(w *meshtest.Writer) { w.Floats("boundingsphere", 1, 2, 3) }),
			"its bounding sphere is 3 values, want 4; ignored",
			nil,
		},
		{
			"a box of the wrong length",
			shape().Object(3, "mesh").Floats("p", quad.Positions...).Ints("tri", 0, 1, 2).Object(4, "aabb").Floats("min", 1),
			"min is 1 values, want 3; ignored",
			nil,
		},
		{
			"a locator of the wrong shape",
			meshtest.New().Object(1, "locator").Object(2, "hand").Floats("p", 1).Floats("q", 1),
			"locator hand: its position is 1 values, want 3; ignored",
			nil,
		},
		{
			"objects nobody knows",
			meshtest.New().Object(1, "future").Object(1, "object").Object(2, "s").Mesh(quad, "").Object(4, "cloth").Object(3, "physics"),
			"object future at the top of the file is not one this reads; ignored",
			meshes(1),
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			file := read(t, test.file.Bytes())
			expectWarning(t, file, test.warning)

			if test.check != nil {
				test.check(t, file)
			}
		})
	}
}

func shape() *meshtest.Writer {
	return meshtest.New().Object(1, "object").Object(2, "testShape")
}

func meshes(want int) func(t *testing.T, file *mesh.File) {
	return func(t *testing.T, file *mesh.File) {
		t.Helper()

		got := 0
		for _, shape := range file.Shapes {
			got += len(shape.Meshes)
		}

		if got != want {
			t.Errorf("%d meshes, want %d", got, want)
		}
	}
}

// skinOf stores one bone for each vertex, in the four slots a skin keeps per
// vertex.
func skinOf(bones ...int32) []int32 {
	var joints []int32
	for _, bone := range bones {
		joints = append(joints, bone, -1, -1, -1)
	}

	return joints
}

// weightsOf gives each of the vertices all of its one bone.
func weightsOf(vertices int) []float32 {
	var weights []float32
	for range vertices {
		weights = append(weights, 1, 0, 0, 0)
	}

	return weights
}

func noSkin(t *testing.T, file *mesh.File) {
	t.Helper()

	if mesh := file.Shapes[0].Meshes[0]; mesh.Skin != nil {
		t.Errorf("skin = %+v, want it left out", mesh.Skin)
	}
}

func TestDecodeKeepsTheTree(t *testing.T) {
	root, err := mesh.Decode(meshtest.New().
		Object(1, "a").Strings("names", "first", "", "third").
		Object(2, "b").Ints("n", 1, 2).
		Object(3, "c").
		Object(2, "d").Floats("f").
		Object(1, "e").Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if version, ok := root.Property("pdxasset"); !ok || version.Kind != mesh.Ints || version.Len() != 2 {
		t.Errorf("version = %+v", version)
	}

	if len(root.Children) != 2 || root.Children[1].Name != "e" {
		t.Fatalf("top level = %+v", root.Children)
	}

	a := root.Children[0]
	if names, _ := a.Property("names"); !slices.Equal(names.Strings, []string{"first", "", "third"}) {
		t.Errorf("names = %q", names.Strings)
	}

	b, ok := a.Child("b")
	if !ok || len(b.Children) != 1 || b.Children[0].Name != "c" {
		t.Errorf("b = %+v", b)
	}

	if d, ok := a.Child("d"); !ok || len(d.Properties) != 1 || d.Properties[0].Len() != 0 {
		t.Errorf("d = %+v, want an empty property", d)
	}

	if _, ok := a.Child("missing"); ok {
		t.Error("a missing child was found")
	}
}

func TestDecodeRefusesBrokenFiles(t *testing.T) {
	cases := map[string][]byte{
		"empty":                       {},
		"another file":                []byte("PNG\r\n\x1a\n"),
		"a text mesh":                 []byte("pdxasset = { 1 0 }"),
		"an object nested too deep":   meshtest.New().Object(1, "a").Object(3, "c").Bytes(),
		"a name without an end":       meshtest.New().Raw('[', 'a', 'b').Bytes(),
		"a property without a name":   meshtest.New().Raw('!').Bytes(),
		"a property name cut short":   meshtest.New().Raw('!', 5, 'a').Bytes(),
		"a property without a kind":   meshtest.New().Raw('!', 1, 'p').Bytes(),
		"a count cut short":           meshtest.New().Raw('!', 1, 'p', 'f', 1, 0).Bytes(),
		"numbers cut short":           meshtest.New().Property("p", 'f', 3).Raw(0, 0, 0, 0).Bytes(),
		"a count beyond the file":     meshtest.New().Property("p", 'f', 0xffffffff).Bytes(),
		"strings beyond the file":     meshtest.New().Property("s", 's', 1000).Raw(0, 0, 0, 0).Bytes(),
		"a string cut short":          meshtest.New().Property("s", 's', 1).Raw(10, 0, 0, 0, 'a').Bytes(),
		"an unknown kind":             meshtest.New().Property("x", 'q', 0).Bytes(),
		"a stray byte":                meshtest.New().Raw('x').Bytes(),
		"objects nested past the cap": deep(mesh.MaxDepth + 1),
	}

	for name, data := range cases {
		if root, err := mesh.Decode(data); err == nil {
			t.Errorf("%s: decoded as %+v", name, root)
		}

		if file, err := mesh.Read(data); err == nil {
			t.Errorf("%s: read as %+v", name, file)
		}
	}
}

func deep(depth int) []byte {
	writer := meshtest.New()
	for level := 1; level <= depth; level++ {
		writer.Object(level, "o")
	}

	return writer.Bytes()
}

// Every way of cutting a file short is refused, never read past its end.
func TestDecodeRefusesEveryCut(t *testing.T) {
	data := meshtest.New().Object(1, "object").Object(2, "s").Mesh(meshtest.Quad(1, 1), "d.dds").
		Object(4, "skin").Ints("bones", 1).Ints("ix", skinOf(0, 0, 0, 0)...).Floats("w", weightsOf(4)...).Bytes()

	whole, err := mesh.Read(data)
	if err != nil || len(whole.Shapes) != 1 {
		t.Fatalf("the whole file: %+v, %v", whole, err)
	}

	for cut := range len(data) {
		file, err := mesh.Read(data[:cut])
		if err != nil {
			continue
		}

		// A cut that happens to fall between records leaves a shorter file
		// that is still well formed, which may read with less in it.
		if len(file.Shapes) > 1 {
			t.Errorf("cut at %d read %d shapes", cut, len(file.Shapes))
		}
	}
}

func TestMeshesOfManyVertices(t *testing.T) {
	grid := meshtest.Grid(300, 300)
	file := read(t, shape().Mesh(grid, "").Bytes())
	expectClean(t, file)

	if mesh := file.Shapes[0].Meshes[0]; mesh.Vertices() != 301*301 || mesh.Triangles() != 300*300*2 {
		t.Errorf("%d vertices and %d triangles", mesh.Vertices(), mesh.Triangles())
	}
}

// FuzzRead runs the reader over inputs the fuzzer makes up. Run it with
//
//	go test ./mesh -run '^$' -fuzz FuzzRead -fuzztime 60s
func FuzzRead(f *testing.F) {
	quad := meshtest.Quad(1, 1)

	for _, seed := range [][]byte{
		meshtest.QuadFile(1, 1),
		shape().Mesh(quad, "d.dds").Object(4, "skin").Ints("bones", 1).Ints("ix", skinOf(0, 0, 0, 0)...).Floats("w", weightsOf(4)...).
			Object(3, "skeleton").Object(4, "root").Ints("ix", 0).Floats("tx", 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0).Bytes(),
		meshtest.New().Object(1, "locator").Object(2, "l").Floats("p", 1, 2, 3).Floats("q", 0, 0, 0, 1).Strings("pa", "root").Bytes(),
		meshtest.New().Object(1, "object").Floats("lodperc", 10).Object(2, "s").Ints("lod", 1).Mesh(quad, "").Bytes(),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := mesh.Read(data)
		if err != nil {
			return
		}

		checkFile(t, file)
	})
}

// checkFile asserts what Read promises of every file it reads: every mesh it
// hands out can be drawn as it is.
func checkFile(t *testing.T, file *mesh.File) {
	t.Helper()

	for _, shape := range file.Shapes {
		for _, read := range shape.Meshes {
			vertices := read.Vertices()

			if vertices == 0 || len(read.Positions) != vertices*3 || len(read.Indices)%3 != 0 || len(read.Indices) == 0 {
				t.Fatalf("mesh of %d position values and %d indices", len(read.Positions), len(read.Indices))
			}

			for _, index := range read.Indices {
				if int(index) >= vertices {
					t.Fatalf("index %d of %d vertices", index, vertices)
				}
			}

			for name, values := range map[string]struct {
				values []float32
				width  int
			}{"normals": {read.Normals, 3}, "tangents": {read.Tangents, 4}} {
				if values.values != nil && len(values.values) != vertices*values.width {
					t.Fatalf("%s: %d values for %d vertices", name, len(values.values), vertices)
				}
			}

			for set, values := range read.UVs {
				if values != nil && len(values) != vertices*2 {
					t.Fatalf("texture coordinates %d: %d values for %d vertices", set, len(values), vertices)
				}
			}

			if skin := read.Skin; skin != nil {
				if skin.Influences < 1 || skin.Influences > mesh.JointsPerVertex || len(skin.Joints) != vertices*mesh.JointsPerVertex || len(skin.Weights) != len(skin.Joints) {
					t.Fatalf("skin = %+v for %d vertices", skin, vertices)
				}

				for _, joint := range skin.Joints {
					if joint < -1 || int(joint) >= len(shape.Skeleton) {
						t.Fatalf("skin names bone %d of %d", joint, len(shape.Skeleton))
					}
				}
			}
		}

		for index, bone := range shape.Skeleton {
			if bone.Index != index || bone.Parent < -1 || bone.Parent >= index {
				t.Fatalf("bone %d = %+v", index, bone)
			}
		}
	}
}
