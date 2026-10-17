package entity

import (
	"os"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/model"
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

	var loaded, parts, warnings, attached, missing, animations int

	longest, longestOf := 0.0, ""

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
		animations += len(built.Animations)

		if seconds := model.Longest(built.Animations); seconds > longest {
			longest, longestOf = seconds, name
		}

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
	t.Logf("they can play %d animations; the longest of any one entity runs %.1fs, of %s", animations, longest, longestOf)
}

// shippedWithoutMeshFile reports the one mesh file the shipped definitions
// name that is not on disk: the fallback Europa Universalis 5 shows for a mesh
// that is not found, which the engine apparently provides itself.
func shippedWithoutMeshFile(err error) bool {
	return strings.Contains(err.Error(), "mesh_not_found_fallback_object.mesh")
}

// TestLoadInstalledMaleHead loads the male head of Victoria 3's portraits with
// its blend shapes, every one of which fits the head, and the attributes that
// drive them, which the genes set.
func TestLoadInstalledMaleHead(t *testing.T) {
	set, assets := installed(t)

	head, ok := assets.Meshes.Get("male_head_mesh")
	if !ok || head.Origin().File != "gfx/models/portraits/male_head/male_head.asset" || len(head.BlendShapes) == 0 {
		t.Skip("the game has not the male head of Victoria 3")
	}

	built, diagnostics, err := NewLoader(set, assets).Load("male_head_entity")
	if err != nil {
		t.Fatal(err)
	}

	for _, diagnostic := range diagnostics {
		t.Errorf("the male head should load cleanly: %s", diagnostic)
	}

	if len(built.Parts) == 0 {
		t.Fatal("the head has no parts")
	}

	part := built.Parts[0]
	shapes := part.Pieces[0].BlendShapes

	if len(shapes) != len(head.BlendShapes) {
		t.Errorf("%d blend shapes loaded of the %d the mesh names", len(shapes), len(head.BlendShapes))
	}

	driven := map[string]bool{}
	for _, attribute := range part.Attributes {
		driven[attribute.BlendShape] = true
	}

	for _, shape := range shapes {
		if !driven[shape.ID] {
			t.Errorf("no attribute drives blend shape %s", shape.ID)
		}
	}

	// A blend shape of the nose moves the nose, which is a small part of the
	// head: some vertices, not all, and not far.
	for _, shape := range shapes {
		if shape.ID != "male_head_bs_nose_size_max" {
			continue
		}

		moved, furthest := 0, float32(0)

		for vertex := 0; vertex+2 < len(shape.Positions); vertex += 3 {
			length := shape.Positions[vertex]*shape.Positions[vertex] + shape.Positions[vertex+1]*shape.Positions[vertex+1] + shape.Positions[vertex+2]*shape.Positions[vertex+2]
			if length > 1e-6 {
				moved++
			}

			furthest = max(furthest, length)
		}

		if vertices := len(shape.Positions) / 3; moved == 0 || moved > vertices/2 || furthest > 4 {
			t.Errorf("the nose moves %d of %d vertices, the furthest by the root of %g", moved, vertices, furthest)
		}
	}
}

// TestLoadInstalledHeadAnimations loads the male head of Victoria 3's
// portraits with its additive animations, each played by one of its
// attributes, and checks what was read off the files about them: every clip
// is seven frames whose middle is about where the head rests.
func TestLoadInstalledHeadAnimations(t *testing.T) {
	set, assets := installed(t)

	head, ok := assets.Meshes.Get("male_head_mesh")
	if !ok || head.Origin().File != "gfx/models/portraits/male_head/male_head.asset" || len(head.AdditiveAnimations) == 0 {
		t.Skip("the game has not the male head of Victoria 3")
	}

	built, diagnostics, err := NewLoader(set, assets).Load("male_head_entity")
	if err != nil {
		t.Fatal(err)
	}

	for _, diagnostic := range diagnostics {
		t.Errorf("the male head should load cleanly: %s", diagnostic)
	}

	part := built.Parts[0]
	if len(part.Additive) != len(head.AdditiveAnimations) {
		t.Errorf("%d additive animations loaded of the %d the mesh names", len(part.Additive), len(head.AdditiveAnimations))
	}

	played := map[string]float64{}
	for _, attribute := range part.Attributes {
		if attribute.Animation != "" {
			played[attribute.Animation] = attribute.Default
		}
	}

	for _, additive := range part.Additive {
		if additive.Animation.Frames != 7 {
			t.Errorf("%s has %d frames, want 7", additive.ID, additive.Animation.Frames)
		}

		if value, ok := played[additive.ID]; !ok || value != 0.5 {
			t.Errorf("%s is played by no attribute of default 0.5: %v, %v", additive.ID, value, ok)
		}
	}
}
