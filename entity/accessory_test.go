package entity

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/pattern"
)

// The variations of the fixture game: a pattern laid over a surface through
// two of the mask's four channels, and two palettes to colour it with.
const fixtureVariations = `
pattern_textures = {
	name = "silk"
	colormask = "gfx/portraits/accessory_variations/textures/silk_masks.dds"
	normal = "gfx/portraits/accessory_variations/textures/silk_normal.dds"
}

pattern_layout = {
	name = "fabric_layout"
	scale = { min = 0.25 max = 0.25 }
	rotation = { min = -0.4 max = 0.4 }
	offset = { x = 0.5 y = -1 }
}

variation = {
	name = "belt_red"

	pattern = {
		r = { textures = "silk" layout = "fabric_layout" }
		g = { textures = "silk" layout = "fabric_layout" }
	}

	color_palette = { texture = "gfx/portraits/accessory_variations/textures/red.dds" }
	color_palette = { texture = "gfx/portraits/accessory_variations/textures/grey.dds" }
}
`

// fixtureAccessoryAsset draws one mesh with three entities: one whose game
// data names an accessory, one that names none, and one that names a variation
// no file holds.
const fixtureAccessoryAsset = `
pdxmesh = {
	name = "belt_mesh"
	file = "belt.mesh"
	meshsettings = { name = "quadShape" index = 0 shader = "portrait_attachment_pattern" }
}

entity = {
	name = "belt_entity"
	pdxmesh = "belt_mesh"
	game_data = {
		portrait_entity_user_data = {
			portrait_accessory = {
				pattern_mask = "gfx/models/portraits/belt/belt_masks.dds"
				variation = "belt_red"
			}
		}
	}
}

entity = { name = "plain_entity" pdxmesh = "belt_mesh" }

entity = {
	name = "lost_entity"
	pdxmesh = "belt_mesh"
	game_data = {
		portrait_entity_user_data = {
			portrait_accessory = {
				pattern_mask = "gfx/models/portraits/belt/belt_masks.dds"
				variation = "belt_green"
			}
		}
	}
}
`

// accessoryGame writes a game with the fixture's variation, its patterns and
// palettes, and the entity that is coloured with them.
func accessoryGame(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/portraits/belt/belt.asset":                        []byte(fixtureAccessoryAsset),
		"gfx/models/portraits/belt/belt.mesh":                         meshtest.QuadFile(1, 1),
		"gfx/models/portraits/belt/belt_masks.dds":                    solidDDS(red),
		"gfx/portraits/accessory_variations/belts.txt":                []byte(fixtureVariations),
		"gfx/portraits/accessory_variations/textures/silk_masks.dds":  solidDDS(blue),
		"gfx/portraits/accessory_variations/textures/silk_normal.dds": solidDDS(green),
		"gfx/portraits/accessory_variations/textures/red.dds":         solidDDS(red),
		"gfx/portraits/accessory_variations/textures/grey.dds":        solidDDS(green),
	})

	return root
}

func loadBelt(t *testing.T, name string) (*model.Model, []string) {
	t.Helper()

	set := folders.Open([]folders.Source{{Name: "game", Path: accessoryGame(t)}})

	built, diagnostics, err := NewLoader(set, asset.Load(set)).Load(name)
	if err != nil {
		t.Fatal(err)
	}

	var messages []string
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}

	return built, messages
}

// An entity whose game data names a portrait accessory is loaded with it: the
// mask, and every pattern and palette the variation offers to colour it with.
func TestLoadReadsTheAccessory(t *testing.T) {
	built, messages := loadBelt(t, "belt_entity")

	if len(messages) != 0 {
		t.Fatalf("diagnostics = %q, want none", messages)
	}

	accessory := built.Parts[0].Accessory
	if accessory == nil {
		t.Fatal("the part was loaded without its accessory")
	}

	if accessory.Variation != "belt_red" {
		t.Errorf("variation = %q, want belt_red", accessory.Variation)
	}

	if accessory.Mask == nil || [4]byte(accessory.Mask.Data) != [4]byte{255, 0, 0, 255} {
		t.Errorf("mask = %+v, want the red one of gfx/models/portraits/belt", accessory.Mask)
	}

	// The variation offers one pattern and two palettes, and the first of
	// each is what is drawn where nothing was chosen.
	chosen, palette, ok := accessory.Chosen()
	if !ok {
		t.Fatal("the accessory has nothing to draw with")
	}

	if len(accessory.Patterns) != 1 || len(accessory.Palettes) != 2 {
		t.Fatalf("%d patterns and %d palettes, want one and two", len(accessory.Patterns), len(accessory.Palettes))
	}

	if chosen.Description != "silk" {
		t.Errorf("the pattern is described as %q, want silk", chosen.Description)
	}

	if palette.Description != "red.dds" {
		t.Errorf("the palette is described as %q, want red.dds", palette.Description)
	}

	// The channels the pattern names draw its colour mask, placed by the
	// layout the variation names: the middle of the range it gives.
	silk := chosen.Layers[pattern.Red]

	if silk.ColourMask == nil || [4]byte(silk.ColourMask.Data) != [4]byte{0, 0, 255, 255} {
		t.Errorf("the red channel draws %+v, want the silk's own mask", silk.ColourMask)
	}

	if got, want := silk.Placement, (pattern.Placement{Scale: 0.25, Offset: [2]float64{0.5, -1}}); got != want {
		t.Errorf("the red channel places the pattern %+v, want %+v", got, want)
	}

	if chosen.Layers[pattern.Green].Placement != silk.Placement {
		t.Error("the green channel does not place the pattern the way the red one does")
	}

	// A channel the pattern says nothing about draws no pattern of its own,
	// which leaves that channel the colour of the palette alone.
	for _, channel := range []int{pattern.Blue, pattern.Alpha} {
		layer := chosen.Layers[channel]

		if layer.ColourMask != nil {
			t.Errorf("channel %d draws a pattern the variation does not name", channel)
		}

		if layer.Placement.Scale != 1 {
			t.Errorf("channel %d is placed %+v, want it left where the surface puts it", channel, layer.Placement)
		}
	}

	// Where a channel reads the palette: the colours of the four channels
	// are side by side in it, four shades deep.
	if got := model.PaletteUV(pattern.Blue, 16, 2); got != [2]float32{8.5 / 16, 0.25} {
		t.Errorf("the blue channel of a palette 16x2 reads %v, want the ninth pixel of the first row", got)
	}

	// A palette written four pixels wide holds one shade of each channel,
	// which is the layout the games' own notes describe.
	if got := model.PaletteUV(pattern.Blue, 4, 1); got != [2]float32{2.5 / 4, 0.5} {
		t.Errorf("the blue channel of a palette 4x1 reads %v, want the third pixel", got)
	}
}

// An entity whose game data names no accessory is loaded without one, and
// nothing is reported: most entities are not accessories.
func TestLoadWithoutAnAccessory(t *testing.T) {
	built, messages := loadBelt(t, "plain_entity")

	if len(messages) != 0 {
		t.Errorf("diagnostics = %q, want none", messages)
	}

	if built.Parts[0].Accessory != nil {
		t.Errorf("a plain entity was loaded with the accessory %+v", built.Parts[0].Accessory)
	}
}

// An entity that names a variation no file holds is drawn all the same, since
// a portrait accessory colours a model that is already there.
func TestLoadWithAMissingVariation(t *testing.T) {
	built, messages := loadBelt(t, "lost_entity")

	if built.Parts[0].Accessory != nil {
		t.Error("the accessory was read from a variation that is not there")
	}

	if len(built.Parts) != 1 || len(built.Parts[0].Pieces) == 0 || built.Parts[0].Pieces[0].Triangles() == 0 {
		t.Error("the entity itself was left out as well")
	}

	if len(messages) != 1 || !strings.Contains(messages[0], "names the variation belt_green, which is in none of the") {
		t.Errorf("diagnostics = %q, want the missing variation alone reported", messages)
	}
}
