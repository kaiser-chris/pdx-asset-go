package entity

import (
	"os"
	"path/filepath"

	"github.com/kaiser-chris/pdx-parser-go/folders"
)

// EngineFolders returns the folders the engine of a game reads its files
// from, before the game's own: its clausewitz and jomini folders, next to the
// game folder of an installation, which hold textures and meshes the game's
// assets use as well. A folder that is not there is left out.
//
// The folders of a game are opened with these first:
//
//	folders.Open(append(entity.EngineFolders(root), folders.Source{Name: name, Path: root}))
func EngineFolders(gameFolder string) []folders.Source {
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
