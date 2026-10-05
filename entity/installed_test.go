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

	// Europa Universalis 5 has a male body of its own, built differently;
	// what is compared here is Victoria 3's.
	body, ok := assets.Meshes.Get("male_body_mesh")
	if !ok || body.Origin().File != "gfx/models/portraits/male_body/male_body.asset" {
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
		if want.image == nil || want.image.Format != want.format || want.image.Width != want.size {
			t.Errorf("%s = %+v, want %v of %d pixels across", name, want.image, want.format, want.size)
		}
	}

	// Upright, feet on the ground, mirrored across x, which leaves a body
	// that is the same on both sides about as wide either way.
	if built.Min[1] < -1 || built.Max[1] < 120 || built.Min[0] > -30 || built.Max[0] < 30 {
		t.Errorf("the body stands from %v to %v", built.Min, built.Max)
	}
}

// TestLoadInstalledEntities loads every entity of the game that draws a mesh.
// None may fail: the mesh files their definitions name were all found when
// the definitions were checked, so a failure is a gap in this package. What
// is reported while loading, which is mostly textures the engine finds in a
// way this package does not know yet, is counted and logged.
func TestLoadInstalledEntities(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}

	set, assets := installed(t)
	loader := NewLoader(set, assets)

	var loaded, parts, warnings, missingTextures int

	for name := range assets.Entities.All() {
		if _, ok := assets.MeshOf(name); !ok {
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

		for _, diagnostic := range diagnostics {
			if strings.Contains(diagnostic.Message, "in any of the folders") {
				missingTextures++
			}
		}

		// Decoded textures are kept for as long as the loader lives, which
		// for every entity of a game is more memory than a test should hold.
		loader.Forget()
	}

	t.Logf("loaded %d entities with %d parts; %d diagnostics, %d of them textures not next to their asset file",
		loaded, parts, warnings, missingTextures)
}

// shippedWithoutMeshFile reports the one mesh file the shipped definitions
// name that is not on disk: the fallback Europa Universalis 5 shows for a mesh
// that is not found, which the engine apparently provides itself.
func shippedWithoutMeshFile(err error) bool {
	return strings.Contains(err.Error(), "mesh_not_found_fallback_object.mesh")
}
