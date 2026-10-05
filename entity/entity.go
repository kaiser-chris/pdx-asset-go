// Package entity loads the entities of the .asset files as models to draw.
//
// An entity names a pdxmesh, which names a .mesh file and says, shape by
// shape, which textures and which shader each part is drawn with. Loading an
// entity reads all of that from a set of game and mod folders, the way the
// game finds it: definitions from pdx-parser-go's asset package, files through
// its folders package, so that a mod replacing a mesh or a texture is drawn
// with its own.
//
// Loading is plain Go, with nothing on the GPU yet, so it can run off the
// thread that draws; the render package uploads the model it returns.
//
// # Robustness
//
// An entity that cannot be loaded at all, because it does not exist, draws no
// mesh, or its mesh file cannot be found or read, is an error. One whose mesh
// file holds no geometry, only locators for others to attach to, loads as a
// model of no parts. Everything
// short of that is drawn as well as it can be and reported: a texture that is
// missing or cannot be read leaves its part drawn with a neutral stand in, and
// what the mesh file did not add up on is passed on from its warnings.
package entity

import (
	"fmt"
	"os"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// Loader loads entities from one set of folders.
//
// It keeps every texture it has decoded, since the entities of a portrait
// share many of theirs, so one loader should be used for as long as its
// folders do not change. A Loader is not safe for use by several goroutines
// at once.
type Loader struct {
	set    *folders.Set
	assets *asset.Assets

	textures map[string]decodedTexture
}

type decodedTexture struct {
	image *texture.Image
	err   error
}

// NewLoader returns a loader for the entities of the given definitions, read
// from the given folders.
func NewLoader(set *folders.Set, assets *asset.Assets) *Loader {
	return &Loader{set: set, assets: assets, textures: map[string]decodedTexture{}}
}

// Forget drops the decoded textures the loader keeps.
func (l *Loader) Forget() {
	clear(l.textures)
}

// Load loads an entity by its name, with the most detailed shapes of its
// mesh. The diagnostics say what could not be read and was drawn as well as
// it could be.
func (l *Loader) Load(name string) (*model.Model, report.Diagnostics, error) {
	entity, ok := l.assets.Entities.Get(name)
	if !ok {
		return nil, nil, fmt.Errorf("there is no entity %s", name)
	}

	definition, ok := l.assets.MeshOf(name)
	if !ok {
		return nil, nil, fmt.Errorf("entity %s draws no mesh that exists", name)
	}

	collector := &report.Collector{}
	subject := "entity " + name

	file, err := l.readMesh(definition)
	if err != nil {
		return nil, nil, fmt.Errorf("entity %s: %w", name, err)
	}

	for _, warning := range file.warnings {
		collector.Addf(report.SeverityWarning, file.source, file.path, 0, 0, subject, "%s", warning)
	}

	settings := settingsOf(entity, definition)

	built := &model.Model{Name: name}

	for _, shape := range file.shapes {
		if shape.LOD != 0 {
			continue
		}

		for index := range shape.Meshes {
			part := model.Part{
				Name:   shape.Name,
				Pieces: model.Convert(&shape.Meshes[index]),
			}

			chosen, found := settings.find(shape.Name, index)
			if !found {
				collector.Addf(report.SeverityWarning, file.source, file.path, 0, 0, subject,
					"pdxmesh %s has no meshsettings for shape %s; drawn untextured", definition.Key, shape.Name)
			} else {
				part.Shader = chosen.Shader
				part.Textures = l.partTextures(chosen, subject, collector)
			}

			built.Parts = append(built.Parts, part)
		}
	}

	// A mesh file without geometry is not broken: some hold nothing but
	// locators, for entities the game attaches others to. Such an entity
	// loads as a model of no parts.
	built.Bounds()

	return built, collector.Diagnostics, nil
}

// meshFile is a mesh file that has been read, and where it was found.
type meshFile struct {
	shapes   []mesh.Shape
	warnings []string
	path     string
	source   string
}

// readMesh finds and reads the mesh file a pdxmesh names.
func (l *Loader) readMesh(definition *asset.Mesh) (meshFile, error) {
	relative := asset.Resolve(definition.Origin(), definition.File)
	if relative == "" {
		return meshFile{}, fmt.Errorf("pdxmesh %s names no mesh file", definition.Key)
	}

	found, ok := l.set.Find(relative)
	if !ok {
		return meshFile{}, fmt.Errorf("pdxmesh %s names %s, which none of the folders has", definition.Key, relative)
	}

	data, err := os.ReadFile(found.Path)
	if err != nil {
		return meshFile{}, fmt.Errorf("read %s: %w", found.Path, err)
	}

	read, err := mesh.Read(data)
	if err != nil {
		return meshFile{}, fmt.Errorf("read %s: %w", found.Path, err)
	}

	return meshFile{shapes: read.Shapes, warnings: read.Warnings, path: found.Path, source: found.Source}, nil
}

// placedSettings are mesh settings, with where they were written, which is
// what the textures they name are relative to.
type placedSettings struct {
	asset.MeshSettings

	origin database.Origin
}

type settingsList []placedSettings

// settingsOf collects the mesh settings that apply to an entity: those of its
// pdxmesh, overridden by the entity's own, which the game reads in that order.
func settingsOf(entity *asset.Entity, definition *asset.Mesh) settingsList {
	var list settingsList

	for _, settings := range definition.Settings {
		list = append(list, placedSettings{MeshSettings: settings, origin: definition.Origin()})
	}

	for _, settings := range entity.Settings {
		list = append(list, placedSettings{MeshSettings: settings, origin: entity.Origin()})
	}

	return list
}

// find returns the settings for one mesh of a shape: the last that names the
// shape and the mesh's index, or failing that the last that names the shape.
// The shape is matched whatever its case, which the shipped files do not
// always agree on.
func (l settingsList) find(shape string, index int) (placedSettings, bool) {
	var byName *placedSettings

	for position := len(l) - 1; position >= 0; position-- {
		settings := &l[position]
		if !strings.EqualFold(settings.Name, shape) {
			continue
		}

		if settings.Index == index {
			return *settings, true
		}

		if byName == nil {
			byName = settings
		}
	}

	if byName != nil {
		return *byName, true
	}

	return placedSettings{}, false
}

// partTextures loads the three textures a part's settings name.
func (l *Loader) partTextures(settings placedSettings, subject string, collector *report.Collector) model.Textures {
	return model.Textures{
		Diffuse:    l.texture(settings, settings.Diffuse, subject, collector),
		Normal:     l.texture(settings, settings.Normal, subject, collector),
		Properties: l.texture(settings, settings.Specular, subject, collector),
	}
}

// texture finds and decodes one texture, relative to the asset file its
// settings were written in. A texture that cannot be found or read is
// reported, and the part is drawn with a neutral stand in for it.
func (l *Loader) texture(settings placedSettings, name, subject string, collector *report.Collector) *texture.Image {
	if name == "" {
		return nil
	}

	note := func(format string, args ...any) {
		collector.Addf(report.SeverityWarning, settings.origin.Source, settings.origin.Path, settings.origin.Line, 0, subject, format, args...)
	}

	relative := asset.Resolve(settings.origin, name)

	found, ok := l.set.Find(relative)
	if !ok {
		note("texture %s is not at %s in any of the folders; drawn without it", name, relative)

		return nil
	}

	cached, ok := l.textures[found.Path]
	if !ok {
		cached = decode(found.Path)
		l.textures[found.Path] = cached
	}

	if cached.err != nil {
		note("texture %s: %v; drawn without it", name, cached.err)

		return nil
	}

	return cached.image
}

func decode(path string) decodedTexture {
	data, err := os.ReadFile(path)
	if err != nil {
		return decodedTexture{err: err}
	}

	image, err := texture.Decode(path, data)

	return decodedTexture{image: image, err: err}
}
