package entity

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// jawAsset defines a jaw of one skinned quad with additive animations, of
// which one is not there, and an entity whose attributes play them.
const jawAsset = `
pdxmesh = {
	name = "jaw_mesh"
	file = "jaw.mesh"
	additive_animation = { id = "jaw_forward" type = "jaw_forward.anim" }
	additive_animation = { id = "jaw_gone" type = "jaw_gone.anim" }
	meshsettings = { name = "quadShape" index = 0 shader = "portrait_skin_face" }
}

entity = {
	name = "jaw_entity"
	pdxmesh = "jaw_mesh"
	attribute = { name = "jaw_forward" additive_animation = "jaw_forward" default = 0.5 }
	attribute = { name = "jaw_missing" additive_animation = "jaw_nothing" default = 0.5 }
}
`

// skinnedQuadFile is a quad skinned to a single bone at the origin.
func skinnedQuadFile() []byte {
	return meshtest.New().
		Object(1, "object").
		Object(2, "quadShape").
		Mesh(meshtest.Quad(2, 2), "").
		Object(4, "skin").
		Ints("bones", 1).
		Ints("ix", 0, -1, -1, -1, 0, -1, -1, -1, 0, -1, -1, -1, 0, -1, -1, -1).
		Floats("w", 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0).
		Object(3, "skeleton").
		Object(4, "bn_jaw").Ints("ix", 0).Floats("tx", 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0).
		Bytes()
}

// jawClip is an additive clip of three frames moving the jaw's bone forward.
func jawClip() []byte {
	writer := meshtest.New()

	writer.Object(1, "info").Floats("fps", 2).Ints("sa", 3).Ints("j", 1)
	writer.Object(2, "bn_jaw").Strings("sa", "t").Floats("t", 0, 0, -1).Floats("q", 0, 0, 0, -1).Floats("s", 1)
	writer.Object(1, "samples").Floats("t", 0, 0, -1, 0, 0, 0, 0, 0, 1)

	return writer.Bytes()
}

// A skinned mesh is loaded with its additive animations, and its parts with
// the attributes that play them and their defaults; what does not fit is
// reported.
func TestLoadAdditiveAnimations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/jaw/jaw.asset":        []byte(jawAsset),
		"gfx/models/jaw/jaw.mesh":         skinnedQuadFile(),
		"gfx/models/jaw/jaw_forward.anim": jawClip(),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})

	built, diagnostics, err := NewLoader(set, asset.Load(set)).Load("jaw_entity")
	if err != nil {
		t.Fatal(err)
	}

	part := built.Parts[0]

	if len(part.Additive) != 1 || part.Additive[0].ID != "jaw_forward" || part.Additive[0].Animation == nil || part.Additive[0].Animation.Frames != 3 {
		t.Fatalf("additive animations = %+v", part.Additive)
	}

	want := model.Attribute{Name: "jaw_forward", Animation: "jaw_forward", Default: 0.5}
	if len(part.Attributes) != 1 || part.Attributes[0] != want {
		t.Errorf("attributes = %+v, want %+v", part.Attributes, want)
	}

	for _, fragment := range []string{
		"additive animation jaw_gone of pdxmesh jaw_mesh: gfx/models/jaw/jaw_gone.anim is in none of the folders",
		"attribute jaw_missing drives additive animation jaw_nothing, which pdxmesh jaw_mesh does not have",
	} {
		found := false

		for _, diagnostic := range diagnostics {
			found = found || strings.Contains(diagnostic.String(), fragment)
		}

		if !found {
			t.Errorf("no diagnostic mentioning %q in %v", fragment, diagnostics)
		}
	}
}
