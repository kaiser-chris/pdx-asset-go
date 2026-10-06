package entity

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"
)

// Files are the folders a loader reads the files the definitions name from.
// A folders.Set of a game and its mods is one; Folder, a plain folder of
// asset files taken out of a game, is another.
type Files interface {
	// Find finds a file by its path below the folders, with forward
	// slashes.
	Find(relative string) (folders.File, bool)

	// Files lists the files of a folder below the folders.
	Files(folder folders.Folder) ([]folders.File, report.Diagnostics)

	// Layers are the layers the folders are split into, if any.
	Layers() []string
}

// Folder is a plain folder of asset files, such as one a modder keeps
// outside the game, with their meshes and textures next to them or in
// folders around them. A path below it may climb out of it with ../, the
// way an asset file names a mesh in a folder next to its own.
type Folder struct {
	// Path is the folder, and Name what diagnostics call it.
	Path string
	Name string
}

// Find finds a file by its path below the folder.
func (f Folder) Find(relative string) (folders.File, bool) {
	if relative == "" {
		return folders.File{}, false
	}

	found := filepath.Join(f.Path, filepath.FromSlash(relative))

	info, err := os.Stat(found)
	if err != nil || info.IsDir() {
		return folders.File{}, false
	}

	return folders.File{Relative: path.Clean(relative), Path: found, Source: f.Name}, true
}

// Files lists the files of a folder below the folder, of the folder's
// extension, ".txt" without one, in order of name.
func (f Folder) Files(folder folders.Folder) ([]folders.File, report.Diagnostics) {
	extension := folder.Extension
	if extension == "" {
		extension = ".txt"
	}

	root := filepath.Join(f.Path, filepath.FromSlash(folder.Path))

	var found []folders.File

	_ = filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if entry.IsDir() {
			if file != root && !folder.Recursive {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.EqualFold(filepath.Ext(file), extension) {
			return nil
		}

		relative, err := filepath.Rel(f.Path, file)
		if err != nil {
			return nil
		}

		found = append(found, folders.File{Relative: filepath.ToSlash(relative), Path: file, Source: f.Name})

		return nil
	})

	return found, nil
}

// Layers are none: a plain folder is not split into layers.
func (f Folder) Layers() []string {
	return nil
}
