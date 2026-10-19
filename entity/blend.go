package entity

import (
	"fmt"
	"os"
	"slices"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/report"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// The blend shapes of a mesh are each a mesh file of their own, named by the
// pdxmesh:
//
//	pdxmesh = {
//		name = "male_head_mesh"
//		file = "male_head.mesh"
//		blend_shape = { id = "male_head_bs_nose_size_max" type = "blendshapes/male_head_bs_nose_size_max.mesh" }
//	}
//
// A blend shape file holds the mesh as it is morphed fully into the shape:
// the same vertices in the same order, moved, under a shape of a name of its
// own, such as male_head_bs_nose_size_maxShape for the male_headShape of the
// mesh. The meshes of a blend shape file are therefore matched to those of
// the mesh by their place among the most detailed shapes, not by name, and
// one whose vertices are not as many is reported and left out.
//
// An entity says which of its attributes drive which blend shape:
//
//	attribute = { name = "head_bs_nose_size_max" blend_shape = "male_head_bs_nose_size_max" }
//
// which is what the genes of a portrait set.

// blendFile is a blend shape file that has been read: the meshes of its most
// detailed shapes, in order, or why it could not be.
type blendFile struct {
	meshes []*mesh.Mesh
	err    error
}

// blendShapes reads the blend shape files of a pdxmesh, once for every time
// it is drawn, and returns for each mesh of the most detailed shapes of the
// mesh file, in order, the shapes it can be morphed into.
func (l *Loader) blendShapes(job *loading, definition *asset.Mesh, file *meshFile, subject string) [][]model.Shape {
	var meshes []*mesh.Mesh

	for index := range file.shapes {
		if file.shapes[index].LOD != 0 {
			continue
		}

		for position := range file.shapes[index].Meshes {
			meshes = append(meshes, &file.shapes[index].Meshes[position])
		}
	}

	shapes := make([][]model.Shape, len(meshes))

	note := func(format string, args ...any) {
		origin := definition.Origin()
		job.collector.Addf(report.SeverityWarning, origin.Source, origin.Path, origin.Line, 0, subject, format, args...)
	}

	for _, blend := range definition.BlendShapes {
		read := l.readBlendOnce(job, definition, blend)
		if read.err != nil {
			note("blend shape %s: %v; drawn without it", blend.ID, read.err)

			continue
		}

		if len(read.meshes) != len(meshes) {
			note("blend shape %s has %d meshes for the %d of %s; those beyond the fewer are drawn without it",
				blend.ID, len(read.meshes), len(meshes), definition.Key)
		}

		for index := range min(len(read.meshes), len(meshes)) {
			if read.meshes[index].Vertices() != meshes[index].Vertices() {
				note("blend shape %s: mesh %d has %d vertices for the %d of %s; drawn without it",
					blend.ID, index, read.meshes[index].Vertices(), meshes[index].Vertices(), definition.Key)

				continue
			}

			shapes[index] = append(shapes[index], model.Shape{ID: blend.ID, Mesh: read.meshes[index], Data: blend.Data})
		}
	}

	return shapes
}

// readBlendOnce reads a blend shape file, once however many entities draw
// it.
func (l *Loader) readBlendOnce(job *loading, definition *asset.Mesh, blend asset.BlendShape) blendFile {
	relative := asset.Resolve(definition.Origin(), blend.File)

	if read, ok := job.blends[relative]; ok {
		return read
	}

	read := l.readBlend(definition, blend, relative)

	if job.blends == nil {
		job.blends = map[string]blendFile{}
	}

	job.blends[relative] = read

	return read
}

// readBlend finds and reads one blend shape file.
func (l *Loader) readBlend(definition *asset.Mesh, blend asset.BlendShape, relative string) blendFile {
	if relative == "" {
		return blendFile{err: fmt.Errorf("names no mesh file")}
	}

	found, ok := l.set.Find(relative)
	if !ok {
		found, ok = l.byName(definition.Origin(), blend.File)
	}

	if !ok {
		return blendFile{err: fmt.Errorf("%s is in none of the folders", relative)}
	}

	data, err := os.ReadFile(found.Path)
	if err != nil {
		return blendFile{err: err}
	}

	read, err := mesh.Read(data)
	if err != nil {
		return blendFile{err: fmt.Errorf("read %s: %w", found.Path, err)}
	}

	var meshes []*mesh.Mesh

	for index := range read.Shapes {
		if read.Shapes[index].LOD != 0 {
			continue
		}

		for position := range read.Shapes[index].Meshes {
			meshes = append(meshes, &read.Shapes[index].Meshes[position])
		}
	}

	return blendFile{meshes: meshes}
}

// attributesOf lists the attributes of an entity that drive a blend shape or
// an additive animation its mesh has: its own, then those of the entities it
// clones that it does not name itself. An attribute driving one the mesh
// does not have is reported, and left out.
func (l *Loader) attributesOf(job *loading, definitions *asset.Assets, entity *asset.Entity, definition *asset.Mesh, subject string) []model.Attribute {
	var (
		found []model.Attribute
		named []string
	)

	hasShape := func(id string) bool {
		return slices.ContainsFunc(definition.BlendShapes, func(shape asset.BlendShape) bool { return shape.ID == id })
	}

	hasAnimation := func(id string) bool {
		return slices.ContainsFunc(definition.AdditiveAnimations, func(animation asset.Animation) bool { return animation.ID == id })
	}

	for _, holder := range definitions.CloneChain(entity.Key) {
		for _, attribute := range holder.Attributes {
			if attribute.BlendShape == "" && attribute.AdditiveAnimation == "" || slices.Contains(named, attribute.Name) {
				continue
			}

			named = append(named, attribute.Name)

			kind, id, ok := "blend shape", attribute.BlendShape, hasShape(attribute.BlendShape)
			if attribute.BlendShape == "" {
				kind, id, ok = "additive animation", attribute.AdditiveAnimation, hasAnimation(attribute.AdditiveAnimation)
			}

			if !ok {
				origin := holder.Origin()
				job.collector.Addf(report.SeverityWarning, origin.Source, origin.Path, origin.Line, 0, subject,
					"attribute %s drives %s %s, which pdxmesh %s does not have; ignored", attribute.Name, kind, id, definition.Key)

				continue
			}

			found = append(found, model.Attribute{
				Name:       attribute.Name,
				BlendShape: attribute.BlendShape,
				Animation:  attribute.AdditiveAnimation,
				Default:    attribute.Default.Or(0),
			})
		}
	}

	return found
}
