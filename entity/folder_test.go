package entity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// looseAsset is an asset file kept outside a game, the way a modder keeps
// one: some files next to it under the paths of the game they were taken
// from, one in a folder beside it, some not there at all.
const looseAsset = `
pdxmesh = {
	name = "near_mesh"
	file = "near.mesh"
	meshsettings = {
		name = "quadShape"
		index = 0
		texture_diffuse = "gone_diffuse.dds"
		texture_normal = "gone_normal.dds"
		shader = "standard"
	}
}
entity = { name = "near_entity" pdxmesh = "near_mesh" }

pdxmesh = {
	name = "game_mesh"
	file = "gfx/models/buildings/game.mesh"
	meshsettings = {
		name = "quadShape"
		index = 0
		texture_diffuse = "gfx/models/buildings/game_diffuse.png"
		shader = "standard"
	}
}
entity = { name = "game_entity" pdxmesh = "game_mesh" }

pdxmesh = {
	name = "beside_mesh"
	file = "../shared/beside.mesh"
	meshsettings = { name = "quadShape" index = 0 shader = "standard" }
}
entity = { name = "beside_entity" pdxmesh = "beside_mesh" }

pdxmesh = {
	name = "gone_mesh"
	file = "gone.mesh"
}
entity = { name = "gone_entity" pdxmesh = "gone_mesh" }
entity = { name = "elsewhere_entity" pdxmesh = "mesh_of_another_file" }

entity = {
	name = "holder_entity"
	locator = { name = "here" position = { 4 0 0 } }
	attach = { here = "neighbour_entity" }
	attach = { here = "stranger_entity" }
}
`

// neighbourAsset is an asset file next to the loose one, which defines an
// entity the loose one attaches.
const neighbourAsset = `
pdxmesh = { name = "neighbour_mesh" file = "near.mesh" meshsettings = { name = "quadShape" index = 0 shader = "standard" } }
entity = { name = "neighbour_entity" pdxmesh = "neighbour_mesh" }
`

// looseLoader reads the asset file of a folder of loose files the way the
// viewer does, with a loader set to draw what it can of it.
func looseLoader(t *testing.T) *Loader {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "my_mod_models")
	tree(t, dir, map[string][]byte{
		"loose.asset":      []byte(looseAsset),
		"neighbour.asset":  []byte(neighbourAsset),
		"sub/far.asset":    []byte(`entity = { name = "stranger_entity" pdxmesh = "neighbour_mesh" }`),
		"near.mesh":        meshtest.QuadFile(2, 2),
		"game.mesh":        meshtest.QuadFile(2, 2),
		"game_diffuse.png": picture(t, green),
	})
	tree(t, filepath.Join(filepath.Dir(dir), "shared"), map[string][]byte{"beside.mesh": meshtest.QuadFile(1, 1)})

	file := folders.File{Relative: "loose.asset", Path: filepath.Join(dir, "loose.asset"), Source: "my_mod_models"}
	syntax := &report.Collector{}
	parsed := database.ParseFiles([]folders.File{file}, syntax)

	loader := NewLoader(Folder{Path: dir, Name: "my_mod_models"}, asset.Read(parsed[0].Document, file, syntax))
	loader.MissingTexture = texture.Checkerboard()
	loader.EmptyWithoutMesh = true
	loader.ByName = true

	return loader
}

// A missing texture of colour shows as the checkerboard, a missing normal
// map is left out.
func TestLooseMissingTextures(t *testing.T) {
	built, diagnostics, err := looseLoader(t).Load("near_entity")
	if err != nil {
		t.Fatal(err)
	}

	textures := built.Parts[0].Textures
	if textures.Diffuse == nil || textures.Diffuse.Width != 64 || textures.Diffuse.Data[0] != 255 || textures.Diffuse.Data[1] != 0 {
		t.Errorf("diffuse = %+v, want the checkerboard", textures.Diffuse)
	}

	if textures.Normal != nil {
		t.Error("the missing normal map stands in as the checkerboard")
	}

	if len(diagnostics) != 2 {
		t.Errorf("diagnostics = %v, want both missing textures reported", diagnostics)
	}
}

// What an asset file names under the path of the game it was taken from is
// found by its name next to it.
func TestLooseFilesByName(t *testing.T) {
	built, diagnostics, err := looseLoader(t).Load("game_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 1 || built.Parts[0].Textures.Diffuse == nil || built.Parts[0].Textures.Diffuse.Data[1] != 255 {
		t.Errorf("parts = %+v, want the mesh and its green texture found by name", built.Parts)
	}

	if len(diagnostics) != 0 {
		t.Errorf("diagnostics = %v", diagnostics)
	}
}

// A mesh in a folder beside the asset file's, named by a path that climbs
// out of it, is found there.
func TestLooseFileBeside(t *testing.T) {
	built, _, err := looseLoader(t).Load("beside_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 1 || built.Max[0] != 0.5 {
		t.Errorf("parts = %d, box to %v; want the quad of 1 from the folder beside", len(built.Parts), built.Max)
	}
}

// An entity whose mesh or mesh file is missing is drawn as nothing, and
// reported.
func TestLooseMissingMesh(t *testing.T) {
	built, diagnostics, err := looseLoader(t).Load("gone_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 0 || len(diagnostics) != 1 {
		t.Errorf("parts = %d, diagnostics = %v; want none, and the mesh reported", len(built.Parts), diagnostics)
	}

	// One whose mesh is defined in another asset file, not read with it.
	built, diagnostics, err = looseLoader(t).Load("elsewhere_entity")
	if err != nil || len(built.Parts) != 0 || len(diagnostics) != 1 {
		t.Errorf("parts = %v, diagnostics = %v, %v; want none, and the mesh reported", built, diagnostics, err)
	}

	// A loader of a game fails, as it always has.
	strict := looseLoader(t)
	strict.EmptyWithoutMesh = false

	if _, _, err := strict.Load("gone_entity"); err == nil {
		t.Error("a missing mesh loaded without the option")
	}
}

// An entity attached that the loose file does not define is looked for in the
// asset files next to it, and one not there either is reported.
func TestLooseAttachmentsNextToIt(t *testing.T) {
	built, diagnostics, err := looseLoader(t).Load("holder_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 1 || built.Parts[0].Entity != "neighbour_entity" || built.Max[0] != -3 {
		t.Errorf("parts = %+v, box to %v; want the neighbour's quad, 4 along x", built.Parts, built.Max)
	}

	if len(diagnostics) != 1 || !mentions(diagnostics, "entity holder_entity attaches stranger_entity, which is not defined, nor in the asset files next to its own; drawn without it") {
		t.Errorf("diagnostics = %v, want the stranger reported, whose file is not next to it", diagnostics)
	}

	if len(built.Attached) != 2 || built.Attached[0].Missing || !built.Attached[1].Missing {
		t.Errorf("attached = %+v", built.Attached)
	}
}

// A plain folder lists its files of an extension, below it or only in it.
func TestFolderFiles(t *testing.T) {
	dir := t.TempDir()
	tree(t, dir, map[string][]byte{"a.asset": nil, "b.txt": nil, "sub/c.asset": nil})

	folder := Folder{Path: dir, Name: "loose"}

	if files, _ := folder.Files(folders.Folder{Extension: ".asset"}); len(files) != 1 || files[0].Relative != "a.asset" {
		t.Errorf("files = %+v, want a.asset", files)
	}

	if files, _ := folder.Files(folders.Folder{Extension: ".asset", Recursive: true}); len(files) != 2 {
		t.Errorf("files = %+v, want both", files)
	}

	if _, ok := folder.Find("sub"); ok {
		t.Error("a folder was found as a file")
	}

	if _, err := os.Stat(filepath.Join(dir, "sub", "c.asset")); err != nil {
		t.Fatal(err)
	}

	if found, ok := folder.Find("sub/c.asset"); !ok || found.Source != "loose" {
		t.Errorf("found = %+v, %v", found, ok)
	}
}
