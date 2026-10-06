package render

import (
	"fmt"
	"runtime"
	"slices"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mat"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/skin"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// Model is a model on the GPU.
type Model struct {
	renderer *Renderer

	// Name, Min and Max are those of the model it was uploaded from.
	Name     string
	Min, Max [3]float32

	parts    []gpuPart
	textures []rl.Texture2D

	// lastPose is what the model was last posed to, so that a model standing
	// still is not re-posed every frame.
	lastPose        *anim.Animation
	lastSeconds     float64
	lastLoop        bool
	lastAttachments []int

	// The arrays raylib draws from stay pinned for as long as the model
	// lives: raylib keeps the pointers it was handed, and every draw hands
	// them back to C.
	pinned runtime.Pinner
}

// gpuPart is one part of a model: its pieces and the material they are all
// drawn with.
type gpuPart struct {
	pieces   []*gpuPiece
	material *rl.Material

	// pass is the order the part is drawn in; see model.Part.Pass, and
	// shadowOnly says the part is drawn as nothing but its shadow, which is
	// not drawn at all.
	pass       int
	shadowOnly bool

	// look is how the shader draws the part; see look.go.
	look partLook

	// skeleton and placement are those of the model part, for skinning:
	// skeleton the bones the pieces are skinned to, and placement where the
	// vertices were placed before they were converted. A part without a
	// skeleton does not move. attachment is the number of the attachment the
	// part belongs to, 0 for the model's own entity, which says whether an
	// animation moves it.
	skeleton   []mesh.Bone
	placement  mat.Transform
	attachment int
}

// gpuPiece is one piece of geometry on the GPU.
type gpuPiece struct {
	mesh *rl.Mesh

	// skin holds what moves a skinned piece, and nil for a static one.
	skin *gpuSkin
}

// gpuSkin is the CPU side of a skinned piece: the pose it starts from, the
// arrays the mesh draws from, and the bones that move each vertex.
type gpuSkin struct {
	// rest are the vertices posing starts from, in the converted coordinates
	// the mesh was uploaded in. moved are the arrays the mesh draws from,
	// which posing writes into; they are the ones that are pinned.
	restPositions, restNormals, restTangents    []float32
	movedPositions, movedNormals, movedTangents []float32

	// joints and weights say which bones move each vertex, as the piece's
	// skin does.
	joints  []int32
	weights []float32
}

// Upload hands a model to the GPU. A texture several parts share is uploaded
// once. It needs the OpenGL context.
func (r *Renderer) Upload(source *model.Model) (*Model, error) {
	uploaded := &Model{renderer: r, Name: source.Name, Min: source.Min, Max: source.Max}
	textures := map[*texture.Image]rl.Texture2D{}

	textureOf := func(image *texture.Image, standIn rl.Texture2D) (rl.Texture2D, error) {
		if image == nil {
			return standIn, nil
		}

		if done, ok := textures[image]; ok {
			return done, nil
		}

		done, err := UploadTexture(image)
		if err != nil {
			return rl.Texture2D{}, err
		}

		textures[image] = done
		uploaded.textures = append(uploaded.textures, done)

		return done, nil
	}

	for index := range source.Parts {
		part := &source.Parts[index]

		material := rl.LoadMaterialDefault()
		material.Shader = r.shader

		gpu := gpuPart{
			material:   &material,
			pass:       part.Pass(),
			shadowOnly: part.ShadowOnly,
			look:       lookOf(part),
			skeleton:   part.Skeleton,
			placement:  part.Placement,
			attachment: part.Attachment,
		}

		uploaded.parts = append(uploaded.parts, gpu)

		for slot, chosen := range map[int32]struct {
			image   *texture.Image
			standIn rl.Texture2D
		}{
			rl.MapDiffuse:   {part.Textures.Diffuse, r.white},
			rl.MapSpecular:  {part.Textures.Properties, r.plain},
			rl.MapNormal:    {part.Textures.Normal, r.flat},
			rl.MapRoughness: {part.Textures.Tint, r.white},
		} {
			done, err := textureOf(chosen.image, chosen.standIn)
			if err != nil {
				uploaded.Unload()

				return nil, fmt.Errorf("model %s, part %s: %w", source.Name, part.Name, err)
			}

			rl.SetMaterialTexture(gpu.material, slot, done)
		}

		for index := range part.Pieces {
			piece, err := uploaded.uploadPiece(&part.Pieces[index])
			if err != nil {
				uploaded.Unload()

				return nil, fmt.Errorf("model %s, part %s: %w", source.Name, part.Name, err)
			}

			uploaded.parts[len(uploaded.parts)-1].pieces = append(uploaded.parts[len(uploaded.parts)-1].pieces, piece)
		}
	}

	return uploaded, nil
}

// uploadPiece hands one piece of geometry to the GPU.
func (m *Model) uploadPiece(piece *model.Piece) (*gpuPiece, error) {
	if piece.Vertices() == 0 || piece.Triangles() == 0 {
		return nil, fmt.Errorf("a piece of %d vertices and %d triangles cannot be drawn", piece.Vertices(), piece.Triangles())
	}

	positions, normals, tangents := piece.Positions, piece.Normals, piece.Tangents

	var skinned *gpuSkin

	// A skinned piece draws from buffers posing fills in each frame, which
	// start as the piece's own vertices and are handed to the GPU as
	// dynamic, so they can be changed.
	if len(piece.Joints) > 0 {
		positions = clone(piece.Positions)
		normals = clone(piece.Normals)
		tangents = clone(piece.Tangents)

		skinned = &gpuSkin{
			restPositions:  piece.Positions,
			restNormals:    piece.Normals,
			restTangents:   piece.Tangents,
			movedPositions: positions,
			movedNormals:   normals,
			movedTangents:  tangents,
			joints:         piece.Joints,
			weights:        piece.Weights,
		}
	}

	// The mesh is allocated on its own rather than kept in a larger struct.
	// Handing C a pointer into a larger structure has it check everything
	// that structure holds for pointers the garbage collector may move, and
	// the arrays of a mesh are the only ones that are pinned.
	mesh := &rl.Mesh{
		VertexCount:   int32(piece.Vertices()),
		TriangleCount: int32(piece.Triangles()),
		Vertices:      &positions[0],
		Normals:       &normals[0],
		Tangents:      &tangents[0],
		Texcoords:     &piece.UV0[0],
		Texcoords2:    &piece.UV1[0],
		Indices:       &piece.Indices[0],
	}

	for _, pointer := range []any{mesh.Vertices, mesh.Normals, mesh.Tangents, mesh.Texcoords, mesh.Texcoords2, mesh.Indices} {
		m.pinned.Pin(pointer)
	}

	rl.UploadMesh(mesh, skinned != nil)

	if mesh.VaoID == 0 {
		return nil, fmt.Errorf("the GPU did not take a piece of %d vertices", piece.Vertices())
	}

	return &gpuPiece{mesh: mesh, skin: skinned}, nil
}

// clone copies a slice of numbers.
func clone(values []float32) []float32 {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]float32, len(values))
	copy(cloned, values)

	return cloned
}

// Pose moves the model's skinned parts to the pose of an animation at a time,
// in seconds. Only the parts of the attachments named are moved, by their
// numbers, 0 for the model's own entity; every other part is put back in the
// pose its mesh is stored in, which is where a nil animation leaves them all.
// It has to run on the thread that owns the OpenGL context, after the model
// has been uploaded.
func (m *Model) Pose(animation *anim.Animation, seconds float64, loop bool, attachments []int) {
	if animation == m.lastPose && seconds == m.lastSeconds && loop == m.lastLoop && slices.Equal(attachments, m.lastAttachments) {
		return
	}

	m.lastPose = animation
	m.lastSeconds = seconds
	m.lastLoop = loop
	m.lastAttachments = append(m.lastAttachments[:0], attachments...)

	for index := range m.parts {
		part := &m.parts[index]
		if len(part.skeleton) == 0 {
			continue
		}

		// An attachment an animation does not move is put back where its
		// mesh stores it, in case it was moved before: an entity taken off
		// the animation has to stand still, not stay where it was.
		move := animation != nil && slices.Contains(attachments, part.attachment)

		var converted []mat.Transform

		if move {
			// The skin matrices are made in the coordinates of the mesh
			// file, then conjugated by the placement and mirror the vertices
			// were placed and converted by, to move them in the coordinates
			// the piece holds.
			matrices := skin.Matrices(animation, part.skeleton, seconds, loop)
			converted = conjugate(matrices, part.placement.Then(model.Mirror))
		}

		for _, piece := range part.pieces {
			if piece.skin == nil {
				continue
			}

			skinned := piece.skin

			if !move {
				copy(skinned.movedPositions, skinned.restPositions)
				copy(skinned.movedNormals, skinned.restNormals)
				copy(skinned.movedTangents, skinned.restTangents)
			} else {
				skin.Apply(skinned.restPositions, skinned.restNormals, skinned.restTangents, skinned.joints, skinned.weights, converted,
					skinned.movedPositions, skinned.movedNormals, skinned.movedTangents)
			}

			rl.UpdateMeshBuffer(*piece.mesh, vertexBufferPosition, floatBytes(skinned.movedPositions), 0)
			rl.UpdateMeshBuffer(*piece.mesh, vertexBufferNormal, floatBytes(skinned.movedNormals), 0)
			rl.UpdateMeshBuffer(*piece.mesh, vertexBufferTangent, floatBytes(skinned.movedTangents), 0)
		}
	}
}

// The vertex buffers of a mesh raylib indexes, the shader attribute locations
// it binds them to: positions, then normals and tangents.
const (
	vertexBufferPosition = 0
	vertexBufferNormal   = 2
	vertexBufferTangent  = 4
)

// conjugate moves skin matrices into the coordinates a mesh was converted
// into, by the transform that conversion applied: each skin matrix becomes
// fixed⁻¹·skin·fixed.
func conjugate(matrices []mat.Transform, fixed mat.Transform) []mat.Transform {
	inverse, ok := fixed.Inverse()
	if !ok {
		return matrices
	}

	converted := make([]mat.Transform, len(matrices))
	for index, matrix := range matrices {
		converted[index] = inverse.Then(matrix).Then(fixed)
	}

	return converted
}

// floatBytes views a slice of numbers as its bytes, for raylib.
func floatBytes(values []float32) []byte {
	if len(values) == 0 {
		return nil
	}

	return unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), len(values)*4)
}

// Draw draws the model with a transform. It has to run in 3D mode, between
// raylib's BeginMode3D and EndMode3D, which a Viewer does.
//
// The parts are drawn in the order of their passes, as the games draw
// them: the solid geometry, then the decals of the ground, then those of the
// building over them.
func (m *Model) Draw(transform rl.Matrix) {
	for pass := range 3 {
		for index := range m.parts {
			part := &m.parts[index]
			if part.pass != pass || part.shadowOnly {
				continue
			}

			m.drawPart(part, transform)
		}
	}
}

// Unload releases the model's geometry and textures. It needs the OpenGL
// context.
func (m *Model) Unload() {
	for _, part := range m.parts {
		for _, piece := range part.pieces {
			rl.UnloadMesh(piece.mesh)
		}

		if part.material != nil {
			releaseMaterial(part.material)
		}
	}

	for _, done := range m.textures {
		rl.UnloadTexture(done)
	}

	m.parts = nil
	m.textures = nil
	m.pinned.Unpin()
}
