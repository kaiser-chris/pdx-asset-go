// Package entity loads the entities of the .asset files as models to draw.
//
// An entity names a pdxmesh, which names a .mesh file and says, shape by
// shape, which textures and which shader each part is drawn with. Loading an
// entity reads all of that from a set of game and mod folders, the way the
// game finds it: definitions from pdx-parser-go's asset package, files through
// its folders package, so that a mod replacing a mesh or a texture is drawn
// with its own.
//
// # Textures
//
// A texture is looked for next to the asset file that names it, the way
// asset.Resolve says. Many are not there: a building names the decal it
// shares with others by its bare file name, kept in a folder of decals
// elsewhere. The engine finds those through a lookup of every DDS file below
// gfx/models by its name, which it builds when it starts, and so does the
// loader. See lookup.go for how it was established.
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
	"errors"
	"fmt"
	"os"
	"path"
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
	set    Files
	assets *asset.Assets

	// MissingTexture stands in for a texture of colour the settings name
	// that cannot be found or read, such as texture.Checkerboard, which
	// shows it is missing. Without it the part is drawn without the
	// texture, with the renderer's neutral stand in. A missing normal or
	// properties map is left out either way, which leaves the shape and the
	// light on it as they would be.
	MissingTexture *texture.Image

	// EmptyWithoutMesh loads an entity whose mesh is not defined, or whose
	// mesh file cannot be found, as a model of no parts, and reports it,
	// rather than failing.
	EmptyWithoutMesh bool

	// ByName looks for a mesh or texture that is not where its asset file
	// says by its file name next to that asset file, the way files taken
	// out of a game are often kept together.
	ByName bool

	textures map[string]decodedTexture

	// lookup finds textures below gfx/models by name. It is built the first
	// time a texture is not next to its asset file.
	lookup *textureLookup
}

type decodedTexture struct {
	image *texture.Image
	err   error
}

// NewLoader returns a loader for the entities of the given definitions, read
// from the given folders.
func NewLoader(set Files, assets *asset.Assets) *Loader {
	return &Loader{set: set, assets: assets, textures: map[string]decodedTexture{}}
}

// Forget drops the decoded textures the loader keeps. The textures below
// gfx/models it found are kept: they belong to the folders, like the
// definitions, and a loader for folders that changed is a new loader.
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

	collector := &report.Collector{}
	subject := "entity " + name

	definition, ok := l.assets.MeshOf(name)
	if !ok && l.EmptyWithoutMesh {
		origin := entity.Origin()
		collector.Addf(report.SeverityWarning, origin.Source, origin.Path, origin.Line, 0, subject, "entity %s draws no mesh that is defined; drawn as nothing", name)

		return &model.Model{Name: name}, collector.Diagnostics, nil
	}

	if !ok {
		return nil, nil, fmt.Errorf("entity %s draws no mesh that exists", name)
	}

	file, err := l.readMesh(definition)
	if errors.Is(err, errNoMeshFile) && l.EmptyWithoutMesh {
		origin := definition.Origin()
		collector.Addf(report.SeverityWarning, origin.Source, origin.Path, origin.Line, 0, subject, "%v; drawn as nothing", err)

		return &model.Model{Name: name}, collector.Diagnostics, nil
	}

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
			source := &shape.Meshes[index]

			part := model.Part{
				Name:   shape.Name,
				Pieces: model.Convert(source),
			}

			chosen, found := settings.find(shape.Name, index)
			if !found {
				collector.Addf(report.SeverityWarning, file.source, file.path, 0, 0, subject,
					"pdxmesh %s has no meshsettings for shape %s; drawn untextured", definition.Key, shape.Name)
			} else {
				part.Shader = chosen.Shader
				part.Subpass = chosen.Subpass
				part.ShadowOnly = chosen.ShadowOnly
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
		found, ok = l.byName(definition.Origin(), definition.File)
	}

	if !ok {
		return meshFile{}, fmt.Errorf("pdxmesh %s names %s, which none of the folders has: %w", definition.Key, relative, errNoMeshFile)
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

// errNoMeshFile is the error of a mesh file that cannot be found.
var errNoMeshFile = errors.New("no such mesh file")

// byName finds a file an asset file names by its file name next to that
// asset file, for a loader that looks there.
func (l *Loader) byName(origin database.Origin, reference string) (folders.File, bool) {
	if !l.ByName {
		return folders.File{}, false
	}

	name := path.Base(strings.ReplaceAll(reference, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return folders.File{}, false
	}

	return l.set.Find(path.Join(path.Dir(origin.File), name))
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
		Diffuse:    l.texture(settings, settings.Diffuse, true, subject, collector),
		Normal:     l.texture(settings, settings.Normal, false, subject, collector),
		Properties: l.texture(settings, settings.Specular, false, subject, collector),
	}
}

// texture finds and decodes one texture: next to the asset file its settings
// were written in, or failing that among the textures below gfx/models by its
// name, the way the engine finds it. A texture that cannot be found or read
// is reported, and the part is drawn with a neutral stand in for it, or, for
// one of colour, with MissingTexture.
func (l *Loader) texture(settings placedSettings, name string, colour bool, subject string, collector *report.Collector) *texture.Image {
	if name == "" {
		return nil
	}

	missing, instead := (*texture.Image)(nil), "drawn without it"
	if colour && l.MissingTexture != nil {
		missing, instead = l.MissingTexture, "drawn with the texture of a missing one"
	}

	note := func(format string, args ...any) {
		collector.Addf(report.SeverityWarning, settings.origin.Source, settings.origin.Path, settings.origin.Line, 0, subject, format, args...)
	}

	relative := asset.Resolve(settings.origin, name)

	found, ok := l.set.Find(relative)
	if !ok {
		found, ok = l.lookUp(settings.origin, name, note)
	}

	if !ok {
		found, ok = l.byName(settings.origin, name)
	}

	switch {
	case ok:
	case looksUp(name):
		note("texture %s is not at %s in any of the folders, nor anywhere below %s; %s", name, relative, lookupFolder, instead)

		return missing
	default:
		note("texture %s is not at %s in any of the folders; %s", name, relative, instead)

		return missing
	}

	cached, ok := l.textures[found.Path]
	if !ok {
		cached = decode(found.Path)
		l.textures[found.Path] = cached
	}

	if cached.err != nil {
		note("texture %s: %v; %s", name, cached.err, instead)

		return missing
	}

	return cached.image
}

// lookUp finds a texture below gfx/models by its name, the way the engine does
// for one that is not next to its asset file.
func (l *Loader) lookUp(origin database.Origin, name string, note func(format string, args ...any)) (folders.File, bool) {
	if l.lookup == nil {
		l.lookup = newTextureLookup(l.set)
	}

	found := l.lookup.find(origin, name)
	if len(found) == 0 {
		return folders.File{}, false
	}

	if len(found) > 1 {
		note("texture %s is below %s %d times; which the game takes is not known, drawn with %s from %s",
			name, lookupFolder, len(found), found[0].Relative, found[0].Source)
	}

	return found[0], true
}

func decode(path string) decodedTexture {
	data, err := os.ReadFile(path)
	if err != nil {
		return decodedTexture{err: err}
	}

	image, err := texture.Decode(path, data)

	return decodedTexture{image: image, err: err}
}
