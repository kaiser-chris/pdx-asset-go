package entity

import (
	"path"
	"slices"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/folders"
)

// The games find most textures next to the asset file that names them, but
// far from all: a building of Victoria 3 names the decal it shares with a
// dozen others by its bare file name, and the file is in a folder of decals
// somewhere else. The engine finds those through a lookup it builds when it
// starts, which it reports in its debug log:
//
//	[pdxassetutil.cpp:304]: ThreadedInitTextureLookup: gfx/models
//	[pdxassetutil.cpp:313]: ThreadedInitTextureLookup found 5610 files
//
// The counts the games log say what is in it, to the file:
//
//   - Victoria 3 logged 5610 with three mods enabled: the DDS files below
//     gfx/models of the game, its DLCs and those mods, each path once, so
//     that a mod's file replacing one of the game's counts once. A PNG does
//     not count.
//   - Europa Universalis 5 logs a lookup each time it mounts its files for
//     another state of the game, and mounts its layers one on top of the
//     other: loading_screen first, then main_menu, then in_game, each layer
//     a root of its own. It logged 16, 19 and 9227 files: the textures of
//     loading_screen, of loading_screen and main_menu, and of all three,
//     DLCs included. The last count holds three Targa files of in_game, so
//     TGA files count as well as DDS files.
//
// The lookup here is built the same way, from the files the folders hand the
// game, and finds a texture by its file name. An asset file of a layer finds
// the textures of that layer and the layers mounted before it.

// lookupFolder is the folder the engine builds its texture lookup from.
const lookupFolder = "gfx/models"

// lookupExtensions are the kinds of file the lookup holds.
var lookupExtensions = []string{".dds", ".tga"}

// layerStack is the order Europa Universalis 5 mounts its layers in, from
// the loading screen to the game itself. A layer sees its own textures and
// those of the layers before it. A layer the games are not known to have is
// taken to see its own textures alone.
var layerStack = []string{"loading_screen", "main_menu", "in_game"}

// textureLookup finds the textures below gfx/models by file name.
type textureLookup struct {
	// byName holds the files of each layer by their name in lower case: the
	// games run on file systems that ignore case, and the shipped files do
	// not always agree with the names on disk.
	byName map[lookupKey][]folders.File

	// layers are the layers of the folders, to tell which layer an asset
	// file is in.
	layers []string
}

type lookupKey struct {
	layer, name string
}

// newTextureLookup lists the textures below gfx/models of a set of folders.
func newTextureLookup(set *folders.Set) *textureLookup {
	lookup := &textureLookup{byName: map[lookupKey][]folders.File{}, layers: set.Layers()}

	for _, extension := range lookupExtensions {
		// The files come in the order the game reads them, each path once,
		// from whichever folder has the last word on it.
		files, _ := set.Files(folders.Folder{Path: lookupFolder, Recursive: true, Extension: extension})

		for _, file := range files {
			key := lookupKey{layer: file.Layer, name: strings.ToLower(path.Base(file.Relative))}
			lookup.byName[key] = append(lookup.byName[key], file)
		}
	}

	return lookup
}

// find returns the textures of a name that an asset file sees. A name that is
// there more than once returns every file of it, the one of the latest folder
// in load order first.
func (l *textureLookup) find(origin database.Origin, reference string) []folders.File {
	if !looksUp(reference) {
		return nil
	}

	name := strings.ToLower(path.Base(strings.ReplaceAll(reference, "\\", "/")))

	var (
		found []folders.File

		// The layers are mounted as roots of their own, so one path in two
		// layers is one file to the engine: the one of the layer mounted
		// last, which is looked at first.
		paths = map[string]bool{}
	)

	visible := visibleLayers(l.layerOf(origin))

	for index := len(visible) - 1; index >= 0; index-- {
		for _, file := range l.byName[lookupKey{layer: visible[index], name: name}] {
			within := strings.ToLower(strings.TrimPrefix(file.Relative, file.Layer+"/"))
			if !paths[within] {
				paths[within] = true
				found = append(found, file)
			}
		}
	}

	// Which of several files of one name the engine takes is not known: the
	// shipped files never have two. A mod is the likely cause, so its file is
	// taken over the game's, and the caller reports the choice.
	slices.SortStableFunc(found, func(a, b folders.File) int { return b.SourceOrder - a.SourceOrder })

	return found
}

// visibleLayers are the layers whose textures an asset file of a layer sees,
// in the order they are mounted: the files outside any layer, then the
// layers of the stack up to and including its own.
func visibleLayers(layer string) []string {
	if layer == "" {
		return []string{""}
	}

	position := slices.Index(layerStack, layer)
	if position < 0 {
		return []string{"", layer}
	}

	return append([]string{""}, layerStack[:position+1]...)
}

// layerOf is the layer an asset file was read from: the first folder of its
// path, when that is one of the layers.
func (l *textureLookup) layerOf(origin database.Origin) string {
	first, _, found := strings.Cut(origin.File, "/")
	if found && slices.Contains(l.layers, first) {
		return first
	}

	return ""
}

// looksUp reports whether a texture is the kind the lookup holds, and so is
// looked for there when it is not next to its asset file. A PNG is only ever
// found next to it.
func looksUp(reference string) bool {
	return slices.ContainsFunc(lookupExtensions, func(extension string) bool {
		return strings.EqualFold(path.Ext(reference), extension)
	})
}
