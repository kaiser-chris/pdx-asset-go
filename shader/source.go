package shader

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaiser-chris/pdx-parser-go/folders"
)

// Engine returns the folders the engine of a game reads its files from,
// before the game's own: its clausewitz and jomini folders, next to the game
// folder in an installation. Most of the shader files are there, such as
// jomini/gfx/FX/jomini/portrait.shader, which the assets name as
// gfx/FX/jomini/portrait.shader. A folder that is not there is left out.
func Engine(gameFolder string) []folders.Source {
	install := filepath.Dir(filepath.Clean(gameFolder))

	var sources []folders.Source

	for _, name := range []string{"clausewitz", "jomini"} {
		folder := filepath.Join(install, name)
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			sources = append(sources, folders.Source{Name: name, Path: folder})
		}
	}

	return sources
}

// Folders reads shader files from a set of folders. In a game split into
// layers, as Europa Universalis 5 is, a file is looked for in the layer the
// asset is in, then in the layers mounted before it: the engine mounts
// loading_screen, then main_menu, then in_game, and a later layer's file of
// a path is the one it reads.
type Folders struct {
	Set *folders.Set

	// Layer is the layer of the asset whose shaders are read, or empty.
	Layer string
}

// layerStack is the order Europa Universalis 5 mounts its layers in.
var layerStack = []string{"loading_screen", "main_menu", "in_game"}

// ReadFile reads a shader file by its path below the game's root.
func (f Folders) ReadFile(name string) ([]byte, error) {
	for _, layer := range f.candidates() {
		relative := name
		if layer != "" {
			relative = layer + "/" + name
		}

		if found, ok := f.Set.Find(relative); ok {
			return os.ReadFile(found.Path)
		}
	}

	return nil, fmt.Errorf("%s, which none of the folders has", name)
}

// candidates are the layers to look in, the one mounted last first.
func (f Folders) candidates() []string {
	if f.Layer == "" || len(f.Set.Layers()) == 0 {
		return []string{""}
	}

	for position, layer := range layerStack {
		if layer == f.Layer {
			candidates := []string{}
			for index := position; index >= 0; index-- {
				candidates = append(candidates, layerStack[index])
			}

			return append(candidates, "")
		}
	}

	return []string{f.Layer, ""}
}
