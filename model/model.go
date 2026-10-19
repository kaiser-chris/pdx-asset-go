// Package model assembles 3D models for drawing: geometry turned into the
// coordinates a graphics library wants and cut into pieces it can index, with
// the textures each part is drawn with.
//
// It is the step between reading the games' files and drawing them. The mesh
// and texture packages read the files as they are; this package turns what
// they read into what the render package uploads, and does it in plain Go, so
// that all of it can be tested without a GPU.
//
// # Coordinates
//
// The games compose their scenes the way Direct3D does, in left handed
// coordinates, and wind the triangles of their meshes anticlockwise around the
// outward normal in right handed terms. OpenGL, which raylib draws with, uses
// right handed coordinates and takes a triangle wound anticlockwise as seen
// from in front as its front face. Convert therefore mirrors every model
// across x and winds its triangles the other way round, which leaves a model
// looking as it does in the game, with its front faces in front, so that
// back face culling works on it.
package model

import (
	"math"
	"strings"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// MaxPieceVertices is how many vertices one piece of geometry may have:
// raylib indexes its meshes with sixteen bit numbers.
const MaxPieceVertices = 65535

// Model is a model ready to be uploaded: its parts, and the box they fit in.
type Model struct {
	Name  string
	Parts []Part

	// Attached are the entities attached to the model, and to those in
	// turn, in the order they were attached, each before what it attaches.
	// Their parts are among Parts, already in place.
	Attached []Attachment

	// Animations are the animations the model's entities can play, those of
	// the model's own entity first. Only what they are is here, not their
	// samples, which are far too large to read for every animation a model
	// has.
	Animations []Animation

	// Min and Max are the corners of the box the model fits in, in the
	// converted coordinates.
	Min, Max [3]float32
}

// Animation is an animation one of a model's entities can play.
//
// The parts it moves are those of its Attachment. What is here is what an
// animation is, so that it can be listed and picked; the samples that put the
// model in its poses are read when one is played, and the render package
// moves the geometry with them.
type Animation struct {
	// ID is the name the mesh refers to the animation by, such as
	// "idle_animation".
	ID string

	// Entity is the entity whose mesh can play the animation, Attachment the
	// number of the attachment that entity is, 0 for the model's own, and
	// Mesh the pdxmesh that names the animation or imports the set it is in.
	Entity     string
	Attachment int
	Mesh       string

	// File is the animation's file, below the folder of the game or mod it
	// was found in.
	File string

	// FPS is the number of samples over the length of the animation, Frames
	// how many samples there are, Joints how many joints they move, and
	// Seconds how long it runs, which is Frames over FPS.
	FPS     float32
	Frames  int
	Joints  int
	Seconds float64
}

// Rate is the rate the animation was made at, in frames a second: the rate at
// which its frames are a frame apart, which is one frame below its samples
// over its length. A single frame has no rate.
func (a Animation) Rate() float64 {
	if a.Frames < 2 || a.Seconds <= 0 {
		return 0
	}

	return float64(a.Frames-1) / a.Seconds
}

// Longest is how long the longest of a list of animations runs, in seconds,
// which is how far a timeline showing all of them has to reach.
func Longest(animations []Animation) float64 {
	longest := 0.0

	for _, animation := range animations {
		longest = max(longest, animation.Seconds)
	}

	return longest
}

// Attachment is an entity attached to another.
//
// An attachment is referred to by its number: 0 for the model's own entity,
// and n for the attachment Attached[n-1].
type Attachment struct {
	// Entity is the entity attached, To the one it is attached to, and
	// Locator the point of that one it hangs from.
	Entity, To, Locator string

	// Parent is the number of the attachment To is: 0 when it is the model's
	// own entity.
	Parent int

	// Missing is set for an attachment that is not drawn: the entity is not
	// defined, or the point it hangs from cannot be found.
	Missing bool
}

// Part is one mesh of a model, drawn with one material.
type Part struct {
	// Name says where the part came from, such as the shape of a mesh file.
	Name string

	// Entity is the entity the part is of: the model's own, or one attached
	// to it, and Attachment the number of that attachment, 0 for the
	// model's own. An entity attached twice has parts of each.
	Entity     string
	Attachment int

	// Pieces are the geometry, cut into pieces raylib can index.
	Pieces []Piece

	// Skeleton is the bones the part's meshes are skinned to, in the order
	// of their indices. It is nil for a part that does not move.
	Skeleton []mesh.Bone

	// SharesPose says the part is posed by the bones of the model's first
	// skinned part rather than by its own: it is the part of an entity that
	// shares the pose of another, as Model.SharePose adds it.
	SharesPose bool

	// Placement is where the part's vertices sit: the attachment placement
	// they were placed by before they were converted, identity for the
	// model's own entity. It is what a skin matrix is conjugated by to move
	// the vertices in the converted coordinates the piece holds.
	Placement mat.Transform

	// Shader names the effect the game draws the part with, such as
	// portrait_skin, which says much of how it is drawn.
	Shader string

	// Subpass is the pass of the renderer the part is drawn in, as its
	// settings say, such as Decals, or empty for the pass of solid geometry.
	Subpass string

	// ShadowOnly is set for a part the game draws as nothing but its
	// shadow.
	ShadowOnly bool

	// Accessory colours and patterns the part, where the entity's game data
	// names a portrait accessory variation. It is nil for a part that is not
	// an accessory, which is most of them.
	Accessory *Accessory

	// DecalBodyPart is the part of a portrait whose decals are laid over the
	// part's skin, such as head or torso, where the entity's game data names
	// one. It is empty for a part no decal is laid over.
	DecalBodyPart string

	// Attributes are the attributes of the part's entity that morph it, each
	// with the blend shape of its pieces or the additive animation of its
	// skeleton it drives.
	Attributes []Attribute

	// Additive are the additive animations the attributes play over the
	// part's skeleton, by ID. They are nil for a part without a skeleton.
	Additive []Additive

	Textures Textures
}

// Attribute is a value the game sets on an entity, such as how fat a
// portrait is or how far forward its chin is. The genes of a portrait's DNA
// set them.
//
// An attribute that drives a blend shape morphs the part towards it by as
// much as its value. One that drives an additive animation picks a place
// along the clip, from its start at 0 to its end at 1, and adds the pose
// there over the part's.
//
// The value is the attribute's Default, plus what the genes add to it: the
// games' additive animations rest at the middle of their clips, which is
// their default of 0.5, and their genes give values from -0.5 to 0.5 around
// it. That is read off the shipped files, which say no more.
type Attribute struct {
	// Name is the attribute, such as head_bs_nose_size_max, and BlendShape
	// the ID of the blend shape it drives, or Animation that of the additive
	// animation.
	Name, BlendShape, Animation string

	// Default is the value of the attribute that nothing sets.
	Default float64
}

// Additive is an additive animation of a part's skeleton.
type Additive struct {
	ID        string
	Animation *anim.Animation
}

// At is the time along an additive animation an attribute's value picks, in
// seconds: 0 at its start, 1 at its last frame, held there beyond either end.
func (a Additive) At(value float64) float64 {
	if a.Animation == nil || a.Animation.FPS <= 0 || a.Animation.Frames < 2 {
		return 0
	}

	return min(max(value, 0), 1) * float64(a.Animation.Frames-1) / float64(a.Animation.FPS)
}

// IsDecal reports whether a part is a decal: drawn after the solid geometry,
// over it, blended by its alpha, in one of the passes the games keep for
// decals.
func (p Part) IsDecal() bool {
	return p.Pass() > 0
}

// Pass is the order a part is drawn in: the solid geometry first, then the
// decals of the ground, which Victoria 3 tiles across the world under its
// buildings, then the decals of a building's own, which lie over those.
func (p Part) Pass() int {
	switch p.Subpass {
	case "Decals":
		return 1
	case "LocalDecals":
		return 2
	}

	return 0
}

// Textures are the textures a part is drawn with. One that is nil is drawn
// with a neutral stand in: white for colour, flat for the normal map.
type Textures struct {
	// Diffuse is the colour, Normal the surface detail, and Properties the
	// material: roughness, metalness and the like, in the channels the games'
	// shaders read them from.
	Diffuse    *texture.Image
	Normal     *texture.Image
	Properties *texture.Image

	// Tint is the colour of a tree's leaves, which the games keep apart
	// from its diffuse map, a grey one. The diffuse map is overlaid with it.
	Tint *texture.Image

	// Occlusion is the colour a portrait part is darkened towards where it
	// is hidden from the light around it, which the games multiply by the
	// part's palette colour: a deep red for skin.
	Occlusion *texture.Image

	// Files say where the diffuse, normal and properties maps were found,
	// for a tool that writes them back, such as an editor of textures.
	Files TextureFiles
}

// TextureFiles are where the textures of a part were found. A texture that
// was not found, or names no file, has a zero TextureFile.
type TextureFiles struct {
	Diffuse, Normal, Properties TextureFile
}

// TextureFile is where one texture was found: the file itself, its path
// below the root of the folder it was found in, with forward slashes, and the
// name of that folder, such as the game's or a mod's.
type TextureFile struct {
	Path, Relative, Source string
}

// Found reports whether the texture was found.
func (f TextureFile) Found() bool {
	return f.Path != ""
}

// Piece is geometry raylib can upload as one mesh: one flat array per
// attribute, and sixteen bit indices.
type Piece struct {
	// Positions and Normals hold three numbers per vertex, Tangents four, the
	// last being which way round the bitangent goes, and UV0 and UV1 two.
	Positions []float32
	Normals   []float32
	Tangents  []float32
	UV0       []float32
	UV1       []float32

	// Indices holds three vertex numbers per triangle.
	Indices []uint16

	// Joints and Weights say which bones move each vertex of a skinned
	// piece, mesh.JointsPerVertex of each, -1 for a bone of no influence.
	// They are nil for a piece that does not move, and align with the
	// vertices above.
	Joints  []int32
	Weights []float32

	// BlendShapes are the shapes the piece can be morphed towards, which
	// align with its vertices too. They are nil for a piece that has none.
	BlendShapes []BlendShape
}

// BlendShape is a shape a piece can be morphed towards, as how far each of
// its vertices goes at full weight: the morphed piece is the piece plus the
// offsets of each of its shapes times its weight. A weight between 0 and 1
// goes part of the way, and a negative one the other way.
type BlendShape struct {
	// ID is the name the shape goes by, which the attributes that drive it
	// refer to.
	ID string

	// Positions and Normals hold three offsets per vertex, and Tangents three
	// too, for the direction of the tangent; which way round the bitangent
	// goes does not change. A normal or tangent blended this way is no longer
	// of unit length, and is to be normalised again.
	Positions []float32
	Normals   []float32
	Tangents  []float32
}

// Shape is a blend shape of a mesh, as Convert takes it: the mesh as it is
// morphed fully into the shape, which has the same vertices in the same
// order, moved. That is how the games store their blend shapes, each in a
// mesh file of its own.
type Shape struct {
	ID   string
	Mesh *mesh.Mesh

	// Data names the vertex data the shape changes, as its asset writes it,
	// such as "normal|tangent", and every kind for none. Europa Universalis
	// 5 gives shapes that only turn the normals and tangents of a face its
	// additive animations move, whose positions are those of another mesh
	// and do not count.
	Data string
}

// changes reports whether the shape changes a kind of vertex data, as Data
// names them: position, normal or tangent.
func (s Shape) changes(kind string) bool {
	if strings.TrimSpace(s.Data) == "" {
		return true
	}

	for _, named := range strings.Split(s.Data, "|") {
		if strings.EqualFold(strings.TrimSpace(named), kind) {
			return true
		}
	}

	return false
}

// Vertices is how many vertices the piece has.
func (p *Piece) Vertices() int {
	return len(p.Positions) / 3
}

// Triangles is how many triangles the piece has.
func (p *Piece) Triangles() int {
	return len(p.Indices) / 3
}

// Mirror is the reflection across x that Convert applies to take the games'
// left handed coordinates to the right handed ones a graphics library wants.
// Moving a converted mesh's vertices conjugates their skin matrices by this,
// after any placement the vertices were placed by.
var Mirror = mat.Transform{Linear: [3][3]float64{{-1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

// Convert turns a mesh into pieces of geometry in right handed coordinates.
//
// Every attribute a piece has is there for every vertex: a mesh without
// normals gets normals worked out from its triangles, and one without
// tangents or texture coordinates gets zeros, which the shaders take as none.
// A skinned mesh carries its joints and weights through, aligned with the
// vertices of each piece.
//
// The shapes become the blend shapes of the pieces, as the offsets of their
// vertices from the mesh's, converted the same way. A shape whose vertices
// are not as many as the mesh's cannot be one of its blend shapes and is left
// out; a shape without normals or tangents leaves those as they are.
func Convert(source *mesh.Mesh, shapes ...Shape) []Piece {
	vertices := source.Vertices()

	positions := make([]float32, len(source.Positions))
	copy(positions, source.Positions)

	normals := make([]float32, vertices*3)
	if len(source.Normals) == vertices*3 {
		copy(normals, source.Normals)
	} else {
		normals = smoothNormals(source.Positions, source.Indices)
	}

	tangents := make([]float32, vertices*4)
	if len(source.Tangents) == vertices*4 {
		copy(tangents, source.Tangents)
	}

	uv0 := filled(source.UV(0), vertices*2)
	uv1 := filled(source.UV(1), vertices*2)

	// Mirroring across x turns every position and direction round. It also
	// turns the handedness of a tangent frame round, so the bitangent, which
	// the shader works out as the cross product of normal and tangent, has to
	// be flipped back.
	for vertex := range vertices {
		positions[vertex*3] = -positions[vertex*3]
		normals[vertex*3] = -normals[vertex*3]
		tangents[vertex*4] = -tangents[vertex*4]
		tangents[vertex*4+3] = -tangents[vertex*4+3]
	}

	// Mirroring also turns the winding of every triangle round, which is
	// undone here by swapping two of its corners.
	indices := make([]uint32, len(source.Indices))
	for triangle := 0; triangle+2 < len(source.Indices); triangle += 3 {
		indices[triangle] = source.Indices[triangle]
		indices[triangle+1] = source.Indices[triangle+2]
		indices[triangle+2] = source.Indices[triangle+1]
	}

	whole := Piece{Positions: positions, Normals: normals, Tangents: tangents, UV0: uv0, UV1: uv1}

	if source.Skin != nil {
		whole.Joints = source.Skin.Joints
		whole.Weights = source.Skin.Weights
	}

	for _, shape := range shapes {
		if blended, ok := offsets(source, shape); ok {
			whole.BlendShapes = append(whole.BlendShapes, blended)
		}
	}

	return split(whole, indices)
}

// offsets works out how far a shape moves each vertex of a mesh, in the
// converted coordinates: the offsets are directions, which mirroring across x
// turns round like the rest.
func offsets(source *mesh.Mesh, shape Shape) (BlendShape, bool) {
	vertices := source.Vertices()
	if shape.Mesh == nil || shape.Mesh.Vertices() != vertices {
		return BlendShape{}, false
	}

	blended := BlendShape{
		ID:        shape.ID,
		Positions: make([]float32, vertices*3),
		Normals:   make([]float32, vertices*3),
		Tangents:  make([]float32, vertices*3),
	}

	if shape.changes("position") {
		for index := range blended.Positions {
			blended.Positions[index] = shape.Mesh.Positions[index] - source.Positions[index]
		}
	}

	if shape.changes("normal") && len(shape.Mesh.Normals) == vertices*3 && len(source.Normals) == vertices*3 {
		for index := range blended.Normals {
			blended.Normals[index] = shape.Mesh.Normals[index] - source.Normals[index]
		}
	}

	if shape.changes("tangent") && len(shape.Mesh.Tangents) == vertices*4 && len(source.Tangents) == vertices*4 {
		for vertex := range vertices {
			for axis := range 3 {
				blended.Tangents[vertex*3+axis] = shape.Mesh.Tangents[vertex*4+axis] - source.Tangents[vertex*4+axis]
			}
		}
	}

	for vertex := range vertices {
		blended.Positions[vertex*3] = -blended.Positions[vertex*3]
		blended.Normals[vertex*3] = -blended.Normals[vertex*3]
		blended.Tangents[vertex*3] = -blended.Tangents[vertex*3]
	}

	return blended, true
}

// filled returns values when it holds the count wanted, and zeros otherwise.
func filled(values []float32, count int) []float32 {
	result := make([]float32, count)
	if len(values) == count {
		copy(result, values)
	}

	return result
}

// split cuts geometry into pieces of at most MaxPieceVertices vertices, each
// with its own copy of the vertices its triangles use. Geometry that fits in
// one piece is kept as it is.
func split(whole Piece, indices []uint32) []Piece {
	if whole.Vertices() <= MaxPieceVertices {
		whole.Indices = make([]uint16, len(indices))
		for index, vertex := range indices {
			whole.Indices[index] = uint16(vertex)
		}

		return []Piece{whole}
	}

	var (
		pieces  []Piece
		current Piece
		mapping map[uint32]uint16
	)

	start := func() {
		current = Piece{}
		mapping = map[uint32]uint16{}

		for _, shape := range whole.BlendShapes {
			current.BlendShapes = append(current.BlendShapes, BlendShape{ID: shape.ID})
		}
	}

	start()

	for triangle := 0; triangle+2 < len(indices); triangle += 3 {
		corners := indices[triangle : triangle+3]

		fresh := 0
		for _, vertex := range corners {
			if _, ok := mapping[vertex]; !ok {
				fresh++
			}
		}

		if current.Vertices()+fresh > MaxPieceVertices {
			pieces = append(pieces, current)
			start()
		}

		for _, vertex := range corners {
			index, ok := mapping[vertex]
			if !ok {
				index = uint16(current.Vertices())
				mapping[vertex] = index
				current.copyVertex(whole, int(vertex))
			}

			current.Indices = append(current.Indices, index)
		}
	}

	if len(current.Indices) > 0 {
		pieces = append(pieces, current)
	}

	return pieces
}

// copyVertex appends one vertex of another piece.
func (p *Piece) copyVertex(from Piece, vertex int) {
	p.Positions = append(p.Positions, from.Positions[vertex*3:vertex*3+3]...)
	p.Normals = append(p.Normals, from.Normals[vertex*3:vertex*3+3]...)
	p.Tangents = append(p.Tangents, from.Tangents[vertex*4:vertex*4+4]...)
	p.UV0 = append(p.UV0, from.UV0[vertex*2:vertex*2+2]...)
	p.UV1 = append(p.UV1, from.UV1[vertex*2:vertex*2+2]...)

	if len(from.Joints) > 0 {
		p.Joints = append(p.Joints, from.Joints[vertex*mesh.JointsPerVertex:(vertex+1)*mesh.JointsPerVertex]...)
	}

	if len(from.Weights) > 0 {
		p.Weights = append(p.Weights, from.Weights[vertex*mesh.JointsPerVertex:(vertex+1)*mesh.JointsPerVertex]...)
	}

	for index, shape := range from.BlendShapes {
		into := &p.BlendShapes[index]
		into.Positions = append(into.Positions, shape.Positions[vertex*3:vertex*3+3]...)
		into.Normals = append(into.Normals, shape.Normals[vertex*3:vertex*3+3]...)
		into.Tangents = append(into.Tangents, shape.Tangents[vertex*3:vertex*3+3]...)
	}
}

// smoothNormals works out a normal for every vertex from the triangles it is
// part of, each weighted by its area, for a mesh whose file has none. The
// triangles are wound as the games wind them, anticlockwise around their
// normal in right handed terms.
func smoothNormals(positions []float32, indices []uint32) []float32 {
	normals := make([]float32, len(positions)/3*3)

	at := func(vertex uint32) [3]float32 {
		return [3]float32{positions[vertex*3], positions[vertex*3+1], positions[vertex*3+2]}
	}

	for triangle := 0; triangle+2 < len(indices); triangle += 3 {
		a, b, c := at(indices[triangle]), at(indices[triangle+1]), at(indices[triangle+2])
		face := cross(sub(b, a), sub(c, a))

		for _, vertex := range indices[triangle : triangle+3] {
			for axis := range 3 {
				normals[vertex*3+uint32(axis)] += face[axis]
			}
		}
	}

	for vertex := 0; vertex+2 < len(normals); vertex += 3 {
		length := float32(math.Sqrt(float64(normals[vertex]*normals[vertex] + normals[vertex+1]*normals[vertex+1] + normals[vertex+2]*normals[vertex+2])))
		if length > 0 {
			normals[vertex] /= length
			normals[vertex+1] /= length
			normals[vertex+2] /= length
		}
	}

	return normals
}

func sub(a, b [3]float32) [3]float32 {
	return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func cross(a, b [3]float32) [3]float32 {
	return [3]float32{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// Bounds works out the box every piece of every part fits in, and stores it
// in the model. A model without vertices fits in an empty box at the origin.
func (m *Model) Bounds() {
	first := true

	for _, part := range m.Parts {
		for _, piece := range part.Pieces {
			for vertex := 0; vertex+2 < len(piece.Positions); vertex += 3 {
				point := [3]float32(piece.Positions[vertex : vertex+3])

				if first {
					m.Min, m.Max, first = point, point, false

					continue
				}

				for axis := range 3 {
					m.Min[axis] = min(m.Min[axis], point[axis])
					m.Max[axis] = max(m.Max[axis], point[axis])
				}
			}
		}
	}

	if first {
		m.Min, m.Max = [3]float32{}, [3]float32{}
	}
}
