package pattern

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"
)

// The files of a mod, written the way the games write them: a pattern's own
// textures, a layout with both a range and a single value, and a variation
// whose pattern names a different texture for each channel.
const (
	variationsFile = `
pattern_textures = {
	name = "silk"
	colormask	= "gfx/portraits/accessory_variations/textures/silk_masks.dds"
	normal		= "gfx/portraits/accessory_variations/textures/silk_normal.dds"
	properties	= "gfx/portraits/accessory_variations/textures/silk_properties.dds"
}

pattern_textures = { name = "trim" colormask = "gfx/portraits/accessory_variations/textures/trim_masks.dds" }

pattern_layout = {
	name = "fabric_layout"
	scale 		= { min = 0.25	max = 0.25 }
	rotation 	= { min = -0.5	max = 0.5 }
	offset 		= { x = { min = 0.25	max = 0.75 } y = { min = -1	max = 1 } }
}

pattern_layout = { name = "plain" scale = 0.1 rotation = 0.0 offset = { x = 0 y = 0 } }

variation = {
	name = "belt_red"

	# Two patterns to choose between, the second weighted twice the first.
	pattern = {
		weight = 1
		r = { textures = "silk" layout = "fabric_layout" }
		g = { textures = "silk" layout = "fabric_layout" }
		b = { textures = "trim" layout = "plain" }
		a = { textures = "silk" layout = "plain" }
	}

	pattern = {
		weight = 2
		g = { textures = "trim" layout = "plain" }
	}

	color_palette = { weight = 1 texture = "gfx/portraits/accessory_variations/textures/red.dds" }
	color_palette = { texture = "gfx/portraits/accessory_variations/textures/grey.dds" }
}
`
	// A variation a mod writes again replaces the one of the game, as the
	// file it is in does.
	replacedFile = `
variation = {
	name = "belt_red"
	color_palette = { weight = 1 texture = "gfx/portraits/accessory_variations/textures/mod.dds" }
}
`
)

// library reads the accessory variations of a game and a mod laid over it.
func library(t *testing.T) *Library {
	t.Helper()

	root := t.TempDir()
	tree(t, filepath.Join(root, "game"), map[string][]byte{
		"gfx/portraits/accessory_variations/belts.txt": []byte(variationsFile),
		// A file of another kind, which is not a variation.
		"gfx/portraits/accessory_variations/notes.md": []byte("nothing here"),
	})
	tree(t, filepath.Join(root, "mod"), map[string][]byte{
		"gfx/portraits/accessory_variations/belts.txt": []byte(replacedFile),
	})

	set := folders.Open([]folders.Source{
		{Name: "game", Path: filepath.Join(root, "game")},
		{Name: "mod", Path: filepath.Join(root, "mod")},
	})

	return Read(set, &report.Collector{})
}

func TestReadVariation(t *testing.T) {
	variation, ok := library(t).Variation("belt_red")
	if !ok {
		t.Fatal("the variation was not read")
	}

	// The mod's file replaced the game's, so the palette of the mod is the
	// one that is there, and the patterns of the game's file are gone with
	// it: a variation is written whole in one file.
	if len(variation.Palettes) != 1 || variation.Palettes[0].Texture != "gfx/portraits/accessory_variations/textures/mod.dds" {
		t.Errorf("palettes = %+v, want the mod's alone", variation.Palettes)
	}
}

// Everything of a variation is read, including the patterns and layouts of a
// file that also says what a layout's ranges are.
func TestReadTheWholeFile(t *testing.T) {
	read := Read(gameFolder(t), &report.Collector{})

	variation, ok := read.Variation("belt_red")
	if !ok {
		t.Fatal("the variation was not read")
	}

	if len(variation.Patterns) != 2 {
		t.Fatalf("%d patterns, want 2", len(variation.Patterns))
	}

	first := variation.Patterns[0]

	if first.Weight != 1 || variation.Patterns[1].Weight != 2 {
		t.Errorf("weights = %g and %g, want 1 and 2", first.Weight, variation.Patterns[1].Weight)
	}

	// Each channel keeps the pattern it names, and the channels a pattern
	// says nothing about are left empty rather than taking another's.
	if got := first.Layer(Red); got.Textures != "silk" || got.Layout != "fabric_layout" {
		t.Errorf("red = %+v, want the silk over the fabric layout", got)
	}

	if got := first.Layer(Blue); got.Textures != "trim" || got.Layout != "plain" {
		t.Errorf("blue = %+v, want the trim, plain", got)
	}

	for _, channel := range []int{Red, Green, Blue, Alpha} {
		if got := variation.Patterns[1].Layer(channel); channel != Green && got != (Layer{}) {
			t.Errorf("the second pattern's channel %d = %+v, want nothing", channel, got)
		}
	}

	if got := variation.Patterns[1].Layer(Green); got.Textures != "trim" {
		t.Errorf("the second pattern's green = %+v, want the trim", got)
	}

	if len(variation.Palettes) != 2 || variation.Palettes[1].Weight != 0 {
		t.Errorf("palettes = %+v, want two, the second of no weight", variation.Palettes)
	}
}

func TestReadTexturesAndLayouts(t *testing.T) {
	read := Read(gameFolder(t), &report.Collector{})

	silk, ok := read.Textures("silk")
	if !ok {
		t.Fatal("the silk pattern was not read")
	}

	if silk.ColourMask != "gfx/portraits/accessory_variations/textures/silk_masks.dds" ||
		silk.Normal != "gfx/portraits/accessory_variations/textures/silk_normal.dds" ||
		silk.Properties != "gfx/portraits/accessory_variations/textures/silk_properties.dds" {
		t.Errorf("silk = %+v", silk)
	}

	// A pattern that names no normal map or properties map still has its
	// mask, which is the pattern itself.
	trim, ok := read.Textures("trim")
	if !ok || trim.ColourMask == "" || trim.Normal != "" || trim.Properties != "" {
		t.Errorf("trim = %+v, %v", trim, ok)
	}

	fabric, ok := read.Layout("fabric_layout")
	if !ok {
		t.Fatal("the fabric layout was not read")
	}

	// A layout written as a range keeps both ends, and the viewer draws the
	// middle of them.
	if fabric.Scale != (Range{Min: 0.25, Max: 0.25}) || fabric.Rotation != (Range{Min: -0.5, Max: 0.5}) {
		t.Errorf("fabric layout = %+v", fabric)
	}

	if got, want := fabric.Pinned(), (Placement{Scale: 0.25, Offset: [2]float64{0.5, 0}}); got != want {
		t.Errorf("the fabric layout is drawn %+v, want %+v", got, want)
	}

	// A layout written as plain numbers is one placement.
	plain, ok := read.Layout("plain")
	if !ok || plain.Pinned() != (Placement{Scale: 0.1}) {
		t.Errorf("plain layout = %+v, %v", plain, ok)
	}
}

// A game with no accessory variations at all, as Europa Universalis 5 has
// none, reads nothing rather than failing.
func TestReadNothing(t *testing.T) {
	set := folders.Open([]folders.Source{{Name: "game", Path: t.TempDir()}})

	read := Read(set, &report.Collector{})

	if !read.Empty() {
		t.Error("a folder of no variations read something")
	}

	if _, ok := read.Variation("belt_red"); ok {
		t.Error("a variation was found in nothing")
	}

	// A library that was never read answers nothing rather than panicking.
	var nothing *Library

	if _, ok := nothing.Variation("belt_red"); ok {
		t.Error("an unread library found a variation")
	}

	if !nothing.Empty() {
		t.Error("an unread library is not empty")
	}
}

// gameFolder opens the files of the game's own file alone, without the mod
// that replaces them, which is what the reader is given to read.
func gameFolder(t *testing.T) *folders.Set {
	t.Helper()

	root := t.TempDir()
	tree(t, filepath.Join(root, "game"), map[string][]byte{
		"gfx/portraits/accessory_variations/belts.txt": []byte(variationsFile),
	})

	return folders.Open([]folders.Source{{Name: "game", Path: filepath.Join(root, "game")}})
}

// tree writes a folder of files, making the folders they are in.
func tree(t *testing.T, root string, files map[string][]byte) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
