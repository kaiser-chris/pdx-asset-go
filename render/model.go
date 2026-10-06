package render

import (
	"fmt"
	"runtime"
	"slices"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/model"
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

	// Problems are the parts drawn with the renderer's own shader because
	// the effect their settings name could not be built, and why.
	Problems []string

	// The arrays raylib draws from stay pinned for as long as the model
	// lives: raylib keeps the pointers it was handed, and every draw hands
	// them back to C.
	pinned runtime.Pinner
}

// gpuPart is one part of a model: its pieces and the material they are all
// drawn with, or the game's effect and the textures it reads.
type gpuPart struct {
	pieces   []*rl.Mesh
	material *rl.Material

	effect *effect

	// shadow is the effect the part casts its shadow with, or nil for none,
	// and shadowOnly says the part is drawn as nothing but its shadow.
	shadow     *effect
	shadowOnly bool

	// pass is the order the part is drawn in; see model.Part.Pass.
	pass int

	// slots are the asset's textures by the slot of the shaders each goes
	// in, files the textures the effect's samplers name for themselves, and
	// white what a sampler reads that has neither.
	slots map[int]rl.Texture2D
	files map[string]rl.Texture2D

	// standIns are what the effect's samplers read that neither the asset
	// nor the sampler gives a texture, by the sampler's name.
	standIns map[string]standIn

	// srgb are the slots whose textures hold colours.
	srgb  map[int]bool
	white rl.Texture2D
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

		gpu := gpuPart{material: &material, white: r.white, pass: part.Pass(), shadowOnly: part.ShadowOnly}

		if r.shaders != nil && part.Shader != "" {
			compiled, err := r.effectFor(part)
			if err != nil {
				uploaded.Problems = append(uploaded.Problems, fmt.Sprintf("part %s: %v; drawn with the viewer's own shader", part.Name, err))
			} else {
				gpu.effect = compiled
				gpu.shadow = r.shadowEffectFor(part)
				gpu.slots = map[int]rl.Texture2D{}
				gpu.srgb = map[int]bool{}
				gpu.files = map[string]rl.Texture2D{}

				for slot := range 16 {
					if image := part.Textures.Slot(slot); image != nil {
						done, err := textureOf(image, r.white)
						if err != nil {
							uploaded.Unload()

							return nil, fmt.Errorf("model %s, part %s: %w", source.Name, part.Name, err)
						}

						gpu.slots[slot] = done
						gpu.srgb[slot] = part.Textures.IsSRGB(slot)
					}
				}

				gpu.standIns = map[string]standIn{}

				bounds := compiled.textures
				if gpu.shadow != nil {
					bounds = append(slices.Clone(bounds), gpu.shadow.textures...)
				}

				for _, bound := range bounds {
					if bound.texture.Sampler != nil && bound.texture.Sampler.File != "" {
						gpu.files[bound.texture.Sampler.File] = r.fileTexture(bound.texture.Sampler.File)
					}

					if bound.texture.Type == "sampler2D" {
						gpu.standIns[bound.texture.Name] = r.standIn(bound.texture)
					}
				}
			}
		}

		uploaded.parts = append(uploaded.parts, gpu)

		for slot, chosen := range map[int32]struct {
			image   *texture.Image
			standIn rl.Texture2D
		}{
			rl.MapDiffuse:  {part.Textures.Diffuse, r.white},
			rl.MapSpecular: {part.Textures.Properties, r.plain},
			rl.MapNormal:   {part.Textures.Normal, r.flat},
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
func (m *Model) uploadPiece(piece *model.Piece) (*rl.Mesh, error) {
	if piece.Vertices() == 0 || piece.Triangles() == 0 {
		return nil, fmt.Errorf("a piece of %d vertices and %d triangles cannot be drawn", piece.Vertices(), piece.Triangles())
	}

	// The mesh is allocated on its own rather than kept in a larger struct.
	// Handing C a pointer into a larger structure has it check everything
	// that structure holds for pointers the garbage collector may move, and
	// the arrays of a mesh are the only ones that are pinned.
	mesh := &rl.Mesh{
		VertexCount:   int32(piece.Vertices()),
		TriangleCount: int32(piece.Triangles()),
		Vertices:      &piece.Positions[0],
		Normals:       &piece.Normals[0],
		Tangents:      &piece.Tangents[0],
		Texcoords:     &piece.UV0[0],
		Texcoords2:    &piece.UV1[0],
		Indices:       &piece.Indices[0],
	}

	for _, pointer := range []any{mesh.Vertices, mesh.Normals, mesh.Tangents, mesh.Texcoords, mesh.Texcoords2, mesh.Indices} {
		m.pinned.Pin(pointer)
	}

	rl.UploadMesh(mesh, false)

	if mesh.VaoID == 0 {
		return nil, fmt.Errorf("the GPU did not take a piece of %d vertices", piece.Vertices())
	}

	return mesh, nil
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

			if part.effect != nil {
				m.drawEffect(part, part.effect, transform)

				continue
			}

			for _, piece := range part.pieces {
				rl.DrawMesh(*piece, *part.material, transform)
			}
		}
	}
}

// drawShadows draws the parts that cast a shadow with the effects they cast
// it with, into the shadow map being drawn.
func (m *Model) drawShadows(transform rl.Matrix) {
	for index := range m.parts {
		if part := &m.parts[index]; part.shadow != nil {
			m.drawEffect(part, part.shadow, transform)
		}
	}
}

// Unload releases the model's geometry and textures. It needs the OpenGL
// context.
func (m *Model) Unload() {
	for _, part := range m.parts {
		for _, piece := range part.pieces {
			rl.UnloadMesh(piece)
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
