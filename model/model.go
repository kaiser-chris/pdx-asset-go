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
// The parts it moves are those of its Attachment, and the geometry is not
// moved by it: the model holds what an animation is, so that it can be
// listed and played, not the pose it puts the model in.
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

	// Shader names the effect the game draws the part with, such as
	// portrait_skin, which says much of how it is drawn.
	Shader string

	// Subpass is the pass of the renderer the part is drawn in, as its
	// settings say, such as Decals, or empty for the pass of solid geometry.
	Subpass string

	// ShadowOnly is set for a part the game draws as nothing but its
	// shadow.
	ShadowOnly bool

	Textures Textures
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
}

// Vertices is how many vertices the piece has.
func (p *Piece) Vertices() int {
	return len(p.Positions) / 3
}

// Triangles is how many triangles the piece has.
func (p *Piece) Triangles() int {
	return len(p.Indices) / 3
}

// Convert turns a mesh into pieces of geometry in right handed coordinates.
//
// Every attribute a piece has is there for every vertex: a mesh without
// normals gets normals worked out from its triangles, and one without
// tangents or texture coordinates gets zeros, which the shaders take as none.
func Convert(source *mesh.Mesh) []Piece {
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

	return split(whole, indices)
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
