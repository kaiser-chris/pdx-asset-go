// Package mesh reads the binary .mesh files of the Paradox games.
//
// A .mesh file is a tree of named objects holding named properties, each
// property an array of numbers or strings. Decode reads that tree as it is;
// Read interprets it as the games lay their models out:
//
//	[object]                    lodperc: the share of the screen of each level of detail
//	  [body_01Shape]            a shape; lod: its level of detail
//	    [mesh]                  one mesh per material of the shape
//	      p n ta u0 u1 u2       positions, normals, tangents, texture coordinates
//	      tri                   the triangles
//	      boundingsphere        the sphere the mesh fits in
//	      [aabb] min max        the box it fits in
//	      [material]            shader, diff, n, spec
//	      [skin]                bones, ix, w: which bones move each vertex, how much
//	    [skeleton]
//	      [bn_root]             a bone: ix, pa, tx
//	[locator]
//	  [pos_1]                   a point to attach to: p, q, pa, tx
//
// The package is plain Go and knows nothing about drawing: positions and
// directions are handed out as the file writes them, in the left handed
// coordinates the games use, and turning them into what a graphics library
// wants is the render package's job.
//
// # Robustness
//
// A file whose structure is broken, cut short or claiming counts it does not
// hold, is refused, since nothing after the break can be trusted. Inside a
// well formed file, data that does not add up is dropped as narrowly as
// possible and listed in the file's Warnings: an attribute of the wrong
// length is left out of its mesh, a triangle naming a vertex that does not
// exist leaves out the mesh, a skin that does not fit its mesh is left out,
// and the rest of the model is still read.
package mesh

import (
	"fmt"
	"math"
)

// The objects and properties the games write.
const (
	objectRoot     = "object"
	objectMesh     = "mesh"
	objectAABB     = "aabb"
	objectMaterial = "material"
	objectSkin     = "skin"
	objectSkeleton = "skeleton"
	objectLocator  = "locator"

	propertyVersion        = "pdxasset"
	propertyLODPercentages = "lodperc"
	propertyLOD            = "lod"
	propertyPositions      = "p"
	propertyNormals        = "n"
	propertyTangents       = "ta"
	propertyTriangles      = "tri"
	propertySphere         = "boundingsphere"
	propertyMin            = "min"
	propertyMax            = "max"
	propertyShader         = "shader"
	propertyDiffuse        = "diff"
	propertyNormalMap      = "n"
	propertySpecular       = "spec"
	propertyInfluences     = "bones"
	propertyJoints         = "ix"
	propertyWeights        = "w"
	propertyIndex          = "ix"
	propertyParent         = "pa"
	propertyTransform      = "tx"
	propertyRotation       = "q"
)

// uvPrefix starts the name of a set of texture coordinates: u0, u1 and so on.
const uvPrefix = 'u'

// JointsPerVertex is how many bones a skin stores for every vertex, whether or
// not it uses them all. Every skinned mesh the games ship stores four.
const JointsPerVertex = 4

// File is everything a mesh file holds.
type File struct {
	// Version is what the file says about the format it is written in. The
	// shipped files all say 1 0.
	Version []int32

	// LODPercentages are the shares of the screen, in percent, at which each
	// level of detail after the first takes over, when the file gives them.
	LODPercentages []float32

	Shapes   []Shape
	Locators []Locator

	// Warnings lists what was left out because it did not add up.
	Warnings []string
}

// Shape is one named part of a model. A model with levels of detail has a
// shape for each.
type Shape struct {
	Name string

	// LOD is the level of detail the shape is, 0 being the most detailed,
	// which is also what a file that does not say means.
	LOD int

	// Meshes are the parts of the shape, one per material.
	Meshes []Mesh

	// Skeleton is the bones the meshes of the shape are skinned to, in the
	// order of their indices. A shape that does not move has none.
	Skeleton []Bone
}

// Mesh is the geometry of one part of a shape, drawn with one material.
//
// The values are laid out the way a graphics library wants them, one flat
// array per attribute, rather than one struct per vertex.
type Mesh struct {
	// Positions holds three numbers per vertex, Normals three, and Tangents
	// four, the last being which way round the bitangent goes. A file that
	// leaves an attribute out leaves it empty here.
	Positions []float32
	Normals   []float32
	Tangents  []float32

	// UVs holds the sets of texture coordinates by their number, two numbers
	// per vertex each. A set the file leaves out is empty.
	UVs [][]float32

	// Indices holds three vertex numbers per triangle.
	Indices []uint32

	// Min and Max are the corners of the box the mesh fits in, and Sphere the
	// centre and radius of the sphere it fits in, as the file gives them.
	Min, Max [3]float32
	Sphere   [4]float32

	Material Material

	// Skin says which bones move each vertex, for a mesh that moves.
	Skin *Skin

	// Extra holds the properties of the mesh this package does not interpret,
	// such as sfs, which some of the shipped files write.
	Extra []Property
}

// Material is what a mesh file says about how a mesh is drawn. The games take
// most of that from the pdxmesh definitions of the .asset files instead,
// which name the shader and the textures again.
type Material struct {
	Shader string

	// The textures, as file names, when the file gives them.
	Diffuse  string
	Normal   string
	Specular string
}

// Skin says which bones move each vertex of a mesh, and how much.
type Skin struct {
	// Influences is how many of the bones stored for a vertex are in use,
	// from 1 to JointsPerVertex. The rest are padding.
	Influences int

	// Joints holds JointsPerVertex bone indices per vertex, -1 for none, and
	// Weights the share each of them has.
	Joints  []int32
	Weights []float32
}

// Bone is one bone of a skeleton.
type Bone struct {
	Name string

	// Index is the number the skin refers to the bone by, and Parent that of
	// the bone it hangs from, or -1 for a root.
	Index  int
	Parent int

	// Transform is what the file stores for the bone: twelve numbers, the
	// inverse of the bone's resting transform as a four by three matrix.
	Transform []float32

	// Extra holds the properties of the bone this package does not
	// interpret.
	Extra []Property
}

// Locator is a named point of a model that other models and effects attach
// to.
type Locator struct {
	Name string

	// Parent is the bone the locator moves with, when it moves with one.
	Parent string

	// Position, and Rotation as a quaternion: x, y, z, w.
	Position [3]float32
	Rotation [4]float32

	// Transform is the matrix the file stores as well, when it does.
	Transform []float32
}

// Vertices is how many vertices the mesh has.
func (m *Mesh) Vertices() int {
	return len(m.Positions) / 3
}

// Triangles is how many triangles the mesh has.
func (m *Mesh) Triangles() int {
	return len(m.Indices) / 3
}

// UV returns a set of texture coordinates, or nothing when the mesh has no
// such set.
func (m *Mesh) UV(set int) []float32 {
	if set < 0 || set >= len(m.UVs) {
		return nil
	}

	return m.UVs[set]
}

// Bounds is the box the mesh's positions fit in, worked out from the
// positions rather than taken from the file.
func (m *Mesh) Bounds() (low, high [3]float32) {
	if len(m.Positions) < 3 {
		return low, high
	}

	low = [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	high = [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}

	for index, value := range m.Positions[:len(m.Positions)/3*3] {
		axis := index % 3
		low[axis] = min(low[axis], value)
		high[axis] = max(high[axis], value)
	}

	return low, high
}

// Bounds is the box every mesh of the file's most detailed shapes fits in.
// It reports false for a file without a mesh.
func (f *File) Bounds() (low, high [3]float32, ok bool) {
	for _, shape := range f.Shapes {
		if shape.LOD != 0 {
			continue
		}

		for index := range shape.Meshes {
			meshLow, meshHigh := shape.Meshes[index].Bounds()

			if !ok {
				low, high, ok = meshLow, meshHigh, true

				continue
			}

			for axis := range 3 {
				low[axis] = min(low[axis], meshLow[axis])
				high[axis] = max(high[axis], meshHigh[axis])
			}
		}
	}

	return low, high, ok
}

// Read reads a binary mesh file.
func Read(data []byte) (*File, error) {
	root, err := Decode(data)
	if err != nil {
		return nil, err
	}

	file := &File{}

	if version, ok := root.Property(propertyVersion); ok {
		file.Version = version.Ints
	}

	for _, object := range root.Children {
		switch object.Name {
		case objectRoot:
			file.readShapes(object)
		case objectLocator:
			file.readLocators(object)
		default:
			file.warn("object %s at the top of the file is not one this reads; ignored", object.Name)
		}
	}

	return file, nil
}

func (f *File) warn(format string, args ...any) {
	f.Warnings = append(f.Warnings, fmt.Sprintf(format, args...))
}

// readShapes reads the shapes of the object that holds them.
func (f *File) readShapes(object *Object) {
	if percentages, ok := object.Property(propertyLODPercentages); ok {
		f.LODPercentages = percentages.Floats
	}

	for _, child := range object.Children {
		shape := Shape{Name: child.Name}

		if lod, ok := child.Property(propertyLOD); ok && len(lod.Ints) == 1 && lod.Ints[0] >= 0 {
			shape.LOD = int(lod.Ints[0])
		} else if ok {
			f.warn("shape %s: its level of detail %v is not one number of at least 0; read as 0", child.Name, lod.Ints)
		}

		for _, part := range child.Children {
			switch part.Name {
			case objectSkeleton:
				shape.Skeleton = f.readSkeleton(child.Name, part)
			case objectMesh:
				// The skin is checked against the skeleton once both are
				// read, since the skeleton comes after the meshes.
			default:
				f.warn("shape %s: object %s is not one this reads; ignored", child.Name, part.Name)
			}
		}

		for _, part := range child.Children {
			if part.Name != objectMesh {
				continue
			}

			label := fmt.Sprintf("shape %s, mesh %d", child.Name, len(shape.Meshes))

			if mesh, ok := f.readMesh(label, part, len(shape.Skeleton)); ok {
				shape.Meshes = append(shape.Meshes, mesh)
			}
		}

		f.Shapes = append(f.Shapes, shape)
	}
}

// readMesh reads one mesh. It reports false for a mesh that cannot be drawn
// at all, which is left out.
func (f *File) readMesh(label string, object *Object, bones int) (Mesh, bool) {
	mesh := Mesh{}

	positions, _ := object.Property(propertyPositions)
	if positions.Kind != Floats || len(positions.Floats) == 0 || len(positions.Floats)%3 != 0 {
		f.warn("%s: %d position values, want three per vertex; the mesh is left out", label, positions.Len())

		return Mesh{}, false
	}

	mesh.Positions = positions.Floats
	vertices := mesh.Vertices()

	triangles, _ := object.Property(propertyTriangles)
	if triangles.Kind != Ints || len(triangles.Ints) < 3 {
		f.warn("%s: it has no triangles; the mesh is left out", label)

		return Mesh{}, false
	}

	whole := len(triangles.Ints) / 3 * 3
	if whole != len(triangles.Ints) {
		f.warn("%s: %d vertex numbers, which is no whole number of triangles; the last %d are left out", label, len(triangles.Ints), len(triangles.Ints)-whole)
	}

	mesh.Indices = make([]uint32, whole)

	for index, vertex := range triangles.Ints[:whole] {
		if vertex < 0 || int(vertex) >= vertices {
			f.warn("%s: a triangle uses vertex %d of %d; the mesh is left out", label, vertex, vertices)

			return Mesh{}, false
		}

		mesh.Indices[index] = uint32(vertex)
	}

	// attribute reads an attribute of the given width per vertex, or leaves
	// it out when it does not match the positions.
	attribute := func(name string, width int) []float32 {
		property, ok := object.Property(name)
		if !ok {
			return nil
		}

		if property.Kind != Floats || len(property.Floats) != vertices*width {
			f.warn("%s: %d values of %s for %d vertices, want %d each; left out", label, property.Len(), name, vertices, width)

			return nil
		}

		return property.Floats
	}

	mesh.Normals = attribute(propertyNormals, 3)
	mesh.Tangents = attribute(propertyTangents, 4)

	for _, property := range object.Properties {
		set, ok := uvSet(property.Name)
		if !ok {
			continue
		}

		for len(mesh.UVs) <= set {
			mesh.UVs = append(mesh.UVs, nil)
		}

		mesh.UVs[set] = attribute(property.Name, 2)
	}

	if sphere, ok := object.Property(propertySphere); ok {
		if sphere.Kind == Floats && len(sphere.Floats) == 4 {
			mesh.Sphere = [4]float32(sphere.Floats)
		} else {
			f.warn("%s: its bounding sphere is %d values, want 4; ignored", label, sphere.Len())
		}
	}

	known := map[string]bool{
		propertyPositions: true, propertyNormals: true, propertyTangents: true,
		propertyTriangles: true, propertySphere: true,
	}

	for _, property := range object.Properties {
		if _, isUV := uvSet(property.Name); !known[property.Name] && !isUV {
			mesh.Extra = append(mesh.Extra, property)
		}
	}

	for _, child := range object.Children {
		switch child.Name {
		case objectAABB:
			mesh.Min = f.vector(label, child, propertyMin)
			mesh.Max = f.vector(label, child, propertyMax)

		case objectMaterial:
			mesh.Material = Material{
				Shader:   text(child, propertyShader),
				Diffuse:  text(child, propertyDiffuse),
				Normal:   text(child, propertyNormalMap),
				Specular: text(child, propertySpecular),
			}

		case objectSkin:
			mesh.Skin = f.readSkin(label, child, vertices, bones)

		default:
			f.warn("%s: object %s is not one this reads; ignored", label, child.Name)
		}
	}

	return mesh, true
}

// uvSet reports which set of texture coordinates a property is, by its name:
// u0 is set 0.
func uvSet(name string) (int, bool) {
	if len(name) < 2 || name[0] != uvPrefix {
		return 0, false
	}

	set := 0

	for _, digit := range name[1:] {
		if digit < '0' || digit > '9' || set > 99 {
			return 0, false
		}

		set = set*10 + int(digit-'0')
	}

	return set, true
}

// vector reads a property of three numbers.
func (f *File) vector(label string, object *Object, name string) [3]float32 {
	property, ok := object.Property(name)
	if !ok {
		return [3]float32{}
	}

	if property.Kind != Floats || len(property.Floats) != 3 {
		f.warn("%s: %s is %d values, want 3; ignored", label, name, property.Len())

		return [3]float32{}
	}

	return [3]float32(property.Floats)
}

// text reads a property holding one string.
func text(object *Object, name string) string {
	property, ok := object.Property(name)
	if !ok || property.Kind != Strings || len(property.Strings) == 0 {
		return ""
	}

	return property.Strings[0]
}

// readSkin reads the skin of a mesh. A skin that does not fit its mesh or its
// skeleton is left out, which draws the mesh unmoved.
func (f *File) readSkin(label string, object *Object, vertices, bones int) *Skin {
	influences, _ := object.Property(propertyInfluences)
	joints, _ := object.Property(propertyJoints)
	weights, _ := object.Property(propertyWeights)

	if influences.Kind != Ints || len(influences.Ints) != 1 || influences.Ints[0] < 1 || influences.Ints[0] > JointsPerVertex {
		f.warn("%s: its skin says each vertex is moved by %v bones, want one number from 1 to %d; the skin is left out", label, influences.Ints, JointsPerVertex)

		return nil
	}

	skin := &Skin{Influences: int(influences.Ints[0])}

	// However many bones are in use, the file stores the same number for
	// every vertex.
	want := vertices * JointsPerVertex

	if joints.Kind != Ints || len(joints.Ints) != want || weights.Kind != Floats || len(weights.Floats) != want {
		f.warn("%s: its skin has %d bone indices and %d weights for %d vertices, want %d each; the skin is left out",
			label, joints.Len(), weights.Len(), vertices, JointsPerVertex)

		return nil
	}

	for _, joint := range joints.Ints {
		if joint < -1 || int(joint) >= bones {
			f.warn("%s: its skin names bone %d of a skeleton of %d; the skin is left out", label, joint, bones)

			return nil
		}
	}

	skin.Joints = joints.Ints
	skin.Weights = weights.Floats

	return skin
}

// readSkeleton reads the bones of a shape. A bone whose index or parent does
// not fit is read as best it can be: in the order written, or as a root.
func (f *File) readSkeleton(shape string, object *Object) []Bone {
	bones := make([]Bone, 0, len(object.Children))

	for position, child := range object.Children {
		bone := Bone{Name: child.Name, Index: position, Parent: -1}
		label := fmt.Sprintf("shape %s, bone %s", shape, child.Name)

		if index, ok := child.Property(propertyIndex); ok {
			if len(index.Ints) == 1 && int(index.Ints[0]) == position {
				bone.Index = int(index.Ints[0])
			} else {
				f.warn("%s: its index %v is not its place %d in the skeleton; read as %d", label, index.Ints, position, position)
			}
		}

		if parent, ok := child.Property(propertyParent); ok {
			// A bone hangs from one that comes before it, which keeps the
			// skeleton a tree.
			if len(parent.Ints) == 1 && parent.Ints[0] >= 0 && int(parent.Ints[0]) < position {
				bone.Parent = int(parent.Ints[0])
			} else {
				f.warn("%s: its parent %v is not a bone before it; read as a root", label, parent.Ints)
			}
		}

		if transform, ok := child.Property(propertyTransform); ok {
			if transform.Kind == Floats && len(transform.Floats) == 12 {
				bone.Transform = transform.Floats
			} else {
				f.warn("%s: its transform is %d values, want 12; ignored", label, transform.Len())
			}
		}

		for _, property := range child.Properties {
			switch property.Name {
			case propertyIndex, propertyParent, propertyTransform:
			default:
				bone.Extra = append(bone.Extra, property)
			}
		}

		bones = append(bones, bone)
	}

	return bones
}

// readLocators reads the locators of the object that holds them.
func (f *File) readLocators(object *Object) {
	for _, child := range object.Children {
		locator := Locator{Name: child.Name, Rotation: [4]float32{0, 0, 0, 1}}
		label := "locator " + child.Name

		if position, ok := child.Property(propertyPositions); ok {
			if position.Kind == Floats && len(position.Floats) == 3 {
				locator.Position = [3]float32(position.Floats)
			} else {
				f.warn("%s: its position is %d values, want 3; ignored", label, position.Len())
			}
		}

		if rotation, ok := child.Property(propertyRotation); ok {
			if rotation.Kind == Floats && len(rotation.Floats) == 4 {
				locator.Rotation = [4]float32(rotation.Floats)
			} else {
				f.warn("%s: its rotation is %d values, want 4; ignored", label, rotation.Len())
			}
		}

		locator.Parent = text(child, propertyParent)

		if transform, ok := child.Property(propertyTransform); ok && transform.Kind == Floats {
			locator.Transform = transform.Floats
		}

		f.Locators = append(f.Locators, locator)
	}
}
