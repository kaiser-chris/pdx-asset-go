package entity

import (
	"os"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

func installed(t *testing.T) (*folders.Set, *asset.Assets) {
	t.Helper()

	game := os.Getenv("PDX_GAME_DIR")
	if game == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	set := folders.Open([]folders.Source{{Name: "game", Path: game}})

	return set, asset.Load(set)
}

// TestLoadInstalledMaleBody loads the male body of Victoria 3's portraits,
// which is what a portrait is built on, and holds it to what its
// files were read by hand to hold.
func TestLoadInstalledMaleBody(t *testing.T) {
	set, assets := installed(t)

	// Europa Universalis 5 has a male body of its own, built differently,
	// and Crusader Kings 3 defines its male body in the same file under the
	// same names, with textures of other formats and sizes. What tells
	// Victoria 3's body apart is the diffuse texture it names; what is
	// compared here is Victoria 3's.
	body, ok := assets.Meshes.Get("male_body_mesh")
	if !ok || body.Origin().File != "gfx/models/portraits/male_body/male_body.asset" ||
		len(body.Settings) == 0 || body.Settings[0].Diffuse != "male_body_01_diffuse.dds" {
		t.Skip("the game has not the male body of Victoria 3")
	}

	built, diagnostics, err := NewLoader(set, assets).Load("male_body_entity")
	if err != nil {
		t.Fatal(err)
	}

	for _, diagnostic := range diagnostics {
		t.Errorf("the male body should load cleanly: %s", diagnostic)
	}

	if len(built.Parts) != 1 || built.Parts[0].Name != "male_bodyShape" || built.Parts[0].Shader != "portrait_skin" {
		t.Fatalf("parts = %+v", built.Parts)
	}

	textures := built.Parts[0].Textures

	for name, want := range map[string]struct {
		image  *texture.Image
		format texture.Format
		size   int
	}{
		"diffuse":    {textures.Diffuse, texture.RGBA8, 2048},
		"normal":     {textures.Normal, texture.RGBA8, 1024},
		"properties": {textures.Properties, texture.DXT5, 2048},
	} {
		if want.image == nil {
			t.Errorf("%s is missing, want %v of %d pixels across", name, want.format, want.size)
		} else if want.image.Format != want.format || want.image.Width != want.size {
			// The image is described by its header alone, as its data would
			// fill the log.
			t.Errorf("%s is %v of %dx%d pixels with %d levels, want %v of %d pixels across",
				name, want.image.Format, want.image.Width, want.image.Height, want.image.Levels, want.format, want.size)
		}
	}

	// Upright, feet on the ground, mirrored across x, which leaves a body
	// that is the same on both sides about as wide either way.
	if built.Min[1] < -1 || built.Max[1] < 120 || built.Min[0] > -30 || built.Max[0] < 30 {
		t.Errorf("the body stands from %v to %v", built.Min, built.Max)
	}
}

// TestLoadInstalledEntities loads every entity of the game that draws a mesh
// or attaches others. None may fail: the mesh files their definitions name were all found when
// the definitions were checked, so a failure is a gap in this package. Nor
// may a texture be missing: the engine finds every one the shipped files
// name, next to its asset file or through its lookup of gfx/models, and so
// must the loader. What else is reported while loading, such as shapes
// without mesh settings or an attachment that cannot be drawn, is counted and
// logged.
func TestLoadInstalledEntities(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}

	set, assets := installed(t)
	loader := NewLoader(set, assets)

	var loaded, parts, warnings, attached, missing int

	for name := range assets.Entities.All() {
		if _, ok := assets.MeshOf(name); !ok && len(attachmentsOf(assets, name)) == 0 {
			continue
		}

		built, diagnostics, err := loader.Load(name)
		if err != nil {
			if !shippedWithoutMeshFile(err) {
				t.Errorf("%v", err)
			}

			continue
		}

		loaded++
		parts += len(built.Parts)
		warnings += len(diagnostics)
		attached += len(built.Attached)

		for _, attachment := range built.Attached {
			if attachment.Missing {
				missing++
			}
		}

		for _, diagnostic := range diagnostics {
			switch {
			case strings.Contains(diagnostic.Message, "texture "):
				t.Errorf("%s", diagnostic)
			case strings.Contains(diagnostic.Message, " attaches "):
				t.Logf("%s", diagnostic)
			}
		}

		// Decoded textures are kept for as long as the loader lives, which
		// for every entity of a game is more memory than a test should hold.
		loader.Forget()
	}

	t.Logf("loaded %d entities with %d parts and %d attachments, %d of them not drawn; %d diagnostics", loaded, parts, attached, missing, warnings)
}

// shippedWithoutMeshFile reports the one mesh file the shipped definitions
// name that is not on disk: the fallback Europa Universalis 5 shows for a mesh
// that is not found, which the engine apparently provides itself.
func shippedWithoutMeshFile(err error) bool {
	return strings.Contains(err.Error(), "mesh_not_found_fallback_object.mesh")
}
