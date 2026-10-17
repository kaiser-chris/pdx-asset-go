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

// headAsset defines a head of one quad with blend shapes, of which one has
// other vertices and one is not there, and entities that drive them.
const headAsset = `
pdxmesh = {
	name = "head_mesh"
	file = "head.mesh"
	blend_shape = { id = "head_bs_wide" type = "blendshapes/head_bs_wide.mesh" }
	blend_shape = { id = "head_bs_grid" type = "blendshapes/head_bs_grid.mesh" }
	blend_shape = { id = "head_bs_gone" type = "blendshapes/head_bs_gone.mesh" }
	meshsettings = { name = "quadShape" index = 0 shader = "portrait_skin_face" }
}

entity = {
	name = "head_entity"
	pdxmesh = "head_mesh"
	attribute = { name = "head_width" blend_shape = "head_bs_wide" }
	attribute = { name = "head_chin" additive_animation = "head_chin" }
	attribute = { name = "head_nothing" blend_shape = "head_bs_nothing" }
}

entity = {
	name = "older_head_entity"
	clone = "head_entity"
	attribute = { name = "head_wider" blend_shape = "head_bs_wide" }
}

entity = {
	name = "seated_entity"
	locator = { name = "neck" position = { 0 10 0 } }
	attach = { neck = "head_entity" }
}
`

func headGame(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/head/head.asset":                    []byte(headAsset),
		"gfx/models/head/head.mesh":                     meshtest.QuadFile(2, 2),
		"gfx/models/head/blendshapes/head_bs_wide.mesh": meshtest.New().Object(1, "object").Object(2, "head_bs_wideShape").Mesh(meshtest.Quad(4, 2), "").Bytes(),
		"gfx/models/head/blendshapes/head_bs_grid.mesh": meshtest.New().Object(1, "object").Object(2, "s").Mesh(meshtest.Grid(3, 3), "").Bytes(),
	})

	return root
}

// A mesh is loaded with the blend shapes that fit it, and an entity with the
// attributes that drive them; what does not fit is reported.
func TestLoadBlendShapes(t *testing.T) {
	set := folders.Open([]folders.Source{{Name: "game", Path: headGame(t)}})
	loader := NewLoader(set, asset.Load(set))

	built, diagnostics, err := loader.Load("head_entity")
	if err != nil {
		t.Fatal(err)
	}

	part := built.Parts[0]

	if shapes := part.Pieces[0].BlendShapes; len(shapes) != 1 || shapes[0].ID != "head_bs_wide" {
		t.Errorf("blend shapes = %+v", shapes)
	}

	if want := []model.Attribute{{Name: "head_width", BlendShape: "head_bs_wide"}}; len(part.Attributes) != 1 || part.Attributes[0] != want[0] {
		t.Errorf("attributes = %+v, want %+v", part.Attributes, want)
	}

	for _, fragment := range []string{
		"blend shape head_bs_grid: mesh 0 has 16 vertices for the 4 of head_mesh",
		"blend shape head_bs_gone: gfx/models/head/blendshapes/head_bs_gone.mesh is in none of the folders",
		"attribute head_nothing drives blend shape head_bs_nothing, which pdxmesh head_mesh does not have",
	} {
		found := false

		for _, diagnostic := range diagnostics {
			found = found || strings.Contains(diagnostic.String(), fragment)
		}

		if !found {
			t.Errorf("no diagnostic mentioning %q in %v", fragment, diagnostics)
		}
	}

	// A clone drives the blend shapes of its own attributes and of those of
	// the entity it clones.
	older, _, err := loader.Load("older_head_entity")
	if err != nil {
		t.Fatal(err)
	}

	if attributes := older.Parts[0].Attributes; len(attributes) != 2 || attributes[0].Name != "head_wider" || attributes[1].Name != "head_width" {
		t.Errorf("attributes of the clone = %+v", attributes)
	}
}

// An attached entity's blend shapes are placed with it: the offsets are
// directions, which its placement turns but does not move.
func TestLoadBlendShapesOfAnAttachment(t *testing.T) {
	set := folders.Open([]folders.Source{{Name: "game", Path: headGame(t)}})

	built, _, err := NewLoader(set, asset.Load(set)).Load("seated_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 1 {
		t.Fatalf("parts = %+v", built.Parts)
	}

	piece := built.Parts[0].Pieces[0]
	offsets := piece.BlendShapes[0].Positions

	// The quad is raised by the locator, and its blend shape still only
	// widens it.
	if piece.Positions[1] != 9 || offsets[0] != 1 || offsets[1] != 0 || offsets[2] != 0 {
		t.Errorf("first corner at %v moves by %v", piece.Positions[:3], offsets[:3])
	}
}
