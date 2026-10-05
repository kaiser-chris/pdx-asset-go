package entity

import (
	"encoding/binary"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// solidDDS is a one pixel DDS file of one colour, uncompressed, which is the
// kind of file the lookup holds.
func solidDDS(fill color.NRGBA) []byte {
	header := make([]byte, 128)
	copy(header, "DDS ")

	put := func(offset int, value uint32) { binary.LittleEndian.PutUint32(header[offset:], value) }

	put(4, 124)       // the size of the header
	put(8, 0x1007)    // caps, height, width and pixel format are set
	put(12, 1)        // height
	put(16, 1)        // width
	put(28, 1)        // one level
	put(76, 32)       // the size of the pixel format
	put(80, 0x40|0x1) // uncompressed, with alpha
	put(88, 32)       // bits a pixel
	put(92, 0x00ff0000)
	put(96, 0x0000ff00)
	put(100, 0x000000ff)
	put(104, 0xff000000)

	return append(header, fill.B, fill.G, fill.R, fill.A)
}

// solidTGA is a one pixel Targa file of one colour, uncompressed, the other
// kind of file the lookup holds.
func solidTGA(fill color.NRGBA) []byte {
	header := make([]byte, 18)
	header[2] = 2                                 // uncompressed true colour
	binary.LittleEndian.PutUint16(header[12:], 1) // width
	binary.LittleEndian.PutUint16(header[14:], 1) // height
	header[16] = 32                               // bits a pixel
	header[17] = 0x28                             // eight bits of alpha, top down

	return append(header, fill.B, fill.G, fill.R, fill.A)
}

// houseAsset names its own diffuse map, and a decal it shares with other
// buildings by its bare name, the way the buildings of Victoria 3 do.
const houseAsset = `
pdxmesh = {
	name = "house_mesh"
	file = "house.mesh"
	meshsettings = {
		name = "quadShape"
		index = 0
		texture_diffuse = "house_diffuse.dds"
		texture_normal = "SHARED_DECAL_NORMAL.dds"
		texture_specular = "shared_properties.png"
		shader = "standard"
	}
}
entity = { name = "house_entity" pdxmesh = "house_mesh" }
`

// houseGame writes a game with the house, the decals it shares in a folder of
// their own, and a texture outside gfx/models.
func houseGame(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/buildings/house/house.asset":       []byte(houseAsset),
		"gfx/models/buildings/house/house.mesh":        meshtest.QuadFile(1, 1),
		"gfx/models/buildings/house/house_diffuse.dds": solidDDS(red),
		"gfx/models/decals/shared_decal_normal.dds":    solidDDS(blue),
		"gfx/models/decals/shared_properties.png":      picture(t, green),
		"gfx/portraits/elsewhere.dds":                  solidDDS(green),
	})

	return root
}

func loadHouse(t *testing.T, sources ...folders.Source) (*model.Model, []string) {
	t.Helper()

	set := folders.Open(sources)

	built, diagnostics, err := NewLoader(set, asset.Load(set)).Load("house_entity")
	if err != nil {
		t.Fatal(err)
	}

	var messages []string
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}

	return built, messages
}

// A DDS file named by its bare name is found below gfx/models wherever it is,
// whatever the case it is written in. A PNG is only found next to its asset
// file, as the engine does not put PNG files in its lookup.
func TestLoadFindsTexturesBelowModelsByName(t *testing.T) {
	built, messages := loadHouse(t, folders.Source{Name: "game", Path: houseGame(t)})

	textures := built.Parts[0].Textures

	if textures.Diffuse == nil || [4]byte(textures.Diffuse.Data) != [4]byte{255, 0, 0, 255} {
		t.Errorf("diffuse = %+v, want the red one next to the asset file", textures.Diffuse)
	}

	if textures.Normal == nil || [4]byte(textures.Normal.Data) != [4]byte{0, 0, 255, 255} {
		t.Errorf("normal = %+v, want the blue decal of the decals folder", textures.Normal)
	}

	if textures.Properties != nil {
		t.Error("the PNG of the decals folder was found by its name, which the engine does not do")
	}

	if len(messages) != 1 || !strings.Contains(messages[0], "shared_properties.png is not at gfx/models/buildings/house/shared_properties.png in any of the folders; drawn") {
		t.Errorf("diagnostics = %q, want the PNG alone reported", messages)
	}
}

// The lookup holds what is below gfx/models, and nothing else.
func TestLoadLooksOnlyBelowModels(t *testing.T) {
	root := houseGame(t)
	tree(t, root, map[string][]byte{
		"gfx/models/buildings/house/house.asset": []byte(strings.Replace(houseAsset, "SHARED_DECAL_NORMAL.dds", "elsewhere.dds", 1)),
	})

	built, messages := loadHouse(t, folders.Source{Name: "game", Path: root})

	if built.Parts[0].Textures.Normal != nil {
		t.Error("a texture of gfx/portraits was found by its name")
	}

	found := false
	for _, message := range messages {
		found = found || strings.Contains(message, "elsewhere.dds is not at gfx/models/buildings/house/elsewhere.dds in any of the folders, nor anywhere below gfx/models")
	}

	if !found {
		t.Errorf("diagnostics = %q, want the texture reported as nowhere below gfx/models", messages)
	}
}

// The game, its DLCs and its mods are one lookup: a mod's file replaces the
// game's at the same path, and the textures of a DLC are found from the game
// as well as the other way round.
func TestLookupSpansTheFolders(t *testing.T) {
	game := houseGame(t)
	mod := filepath.Join(t.TempDir(), "mod")

	tree(t, game, map[string][]byte{
		"dlc/dlc001_houses/gfx/models/dlc_only_diffuse.dds": solidDDS(green),
		"gfx/models/units/material_index.tga":               solidTGA(blue),
	})
	tree(t, mod, map[string][]byte{
		"gfx/models/decals/shared_decal_normal.dds": solidDDS(green),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: game}, {Name: "mod", Path: mod}})
	lookup := newTextureLookup(set)

	// Every DDS and TGA path once, as the engine counts them: the house's
	// diffuse map, the decal the mod replaces, the DLC's own and the Targa
	// file. Neither the PNG nor what is outside gfx/models.
	count := 0
	for _, files := range lookup.byName {
		count += len(files)
	}

	if count != 4 {
		t.Errorf("the lookup holds %d files, want 4", count)
	}

	origin := database.Origin{File: "gfx/models/buildings/house/house.asset"}

	if found := lookup.find(origin, "shared_decal_normal.dds"); len(found) != 1 || found[0].Source != "mod" {
		t.Errorf("decal = %+v, want the mod's alone", found)
	}

	if found := lookup.find(origin, "DLC_ONLY_diffuse.dds"); len(found) != 1 {
		t.Errorf("DLC texture = %+v, want it found from the game", found)
	}

	if found := lookup.find(origin, "../some/folder/house_diffuse.dds"); len(found) != 1 {
		t.Errorf("a reference with folders = %+v, want it found by its name", found)
	}

	if found := lookup.find(origin, "material_index.TGA"); len(found) != 1 {
		t.Errorf("Targa texture = %+v, want it found like a DDS file", found)
	}
}

// Two files of one name, here from a mod that adds its own copy of a decal
// under another path, leave the choice to the lookup: the mod's is taken, as
// the latest in load order, and the choice is reported, since what the
// engine does with two is not known.
func TestLookupReportsTwoFilesOfOneName(t *testing.T) {
	game := houseGame(t)
	mod := filepath.Join(t.TempDir(), "mod")

	tree(t, mod, map[string][]byte{
		"gfx/models/mod_decals/shared_decal_normal.dds": solidDDS(green),
	})

	built, messages := loadHouse(t, folders.Source{Name: "game", Path: game}, folders.Source{Name: "mod", Path: mod})

	if normal := built.Parts[0].Textures.Normal; normal == nil || normal.Data[1] != 255 {
		t.Errorf("normal = %+v, want the mod's green", normal)
	}

	found := false
	for _, message := range messages {
		found = found || strings.Contains(message, "SHARED_DECAL_NORMAL.dds is below gfx/models 2 times; which the game takes is not known, drawn with gfx/models/mod_decals/shared_decal_normal.dds from mod")
	}

	if !found {
		t.Errorf("diagnostics = %q, want the choice reported", messages)
	}
}

// A game split into layers mounts them one on top of the other, the way
// Europa Universalis 5 does: the loading screen, then the main menu, then the
// game. An asset file of a layer sees the textures of that layer and of those
// mounted before it, not those mounted after.
func TestLookupStacksLayers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"loading_screen/gfx/models/debug/nonormal.dds": solidDDS(blue),
		"main_menu/gfx/models/scenes/sky.dds":          solidDDS(green),
		"in_game/gfx/models/decals/in_game.dds":        solidDDS(red),
		"custom/gfx/models/custom.dds":                 solidDDS(red),
	})

	lookup := newTextureLookup(folders.Open([]folders.Source{{Name: "game", Path: root}}))

	for layer, want := range map[string]map[string]bool{
		"in_game":        {"nonormal.dds": true, "sky.dds": true, "in_game.dds": true, "custom.dds": false},
		"main_menu":      {"nonormal.dds": true, "sky.dds": true, "in_game.dds": false, "custom.dds": false},
		"loading_screen": {"nonormal.dds": true, "sky.dds": false, "in_game.dds": false, "custom.dds": false},
		// A layer the games are not known to have sees its own alone.
		"custom": {"nonormal.dds": false, "sky.dds": false, "in_game.dds": false, "custom.dds": true},
	} {
		origin := database.Origin{File: layer + "/gfx/models/house/house.asset"}

		for name, visible := range want {
			if found := lookup.find(origin, name); (len(found) == 1) != visible {
				t.Errorf("from %s, %s = %+v, want found %v", layer, name, found, visible)
			}
		}
	}
}

// One path in two layers is one file to the engine, which mounts the layers
// as roots of their own: the one of the layer mounted last.
func TestLookupMergesLayers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"main_menu/gfx/models/shared/flag.dds": solidDDS(green),
		"in_game/gfx/models/shared/flag.dds":   solidDDS(red),
	})

	lookup := newTextureLookup(folders.Open([]folders.Source{{Name: "game", Path: root}}))

	found := lookup.find(database.Origin{File: "in_game/gfx/models/house/house.asset"}, "flag.dds")
	if len(found) != 1 || found[0].Layer != "in_game" {
		t.Errorf("flag.dds = %+v, want the in_game one alone", found)
	}
}
