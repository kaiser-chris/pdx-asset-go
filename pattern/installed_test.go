package pattern

import (
	"os"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"
)

// TestReadInstalledVariations reads the accessory variations of a real game,
// which is where the patterns and palettes of its portraits are.
//
// It is skipped unless PDX_GAME_DIR points at a game folder. A mod is read
// with it when PDX_MOD_DIR points at one; the variations of the mod are the
// ones that name the patterns of the game, so both together is what says
// whether the files of a mod are read as the game reads them.
func TestReadInstalledVariations(t *testing.T) {
	root := os.Getenv("PDX_GAME_DIR")
	if root == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	sources := []folders.Source{{Name: "game", Path: root}}

	if mod := os.Getenv("PDX_MOD_DIR"); mod != "" {
		sources = append(sources, folders.Source{Name: "mod", Path: mod})
	}

	read := Read(folders.Open(sources), &report.Collector{})

	if read.Empty() {
		t.Fatalf("%s holds no accessory variations", root)
	}

	variations, patterns, layouts := 0, 0, 0

	for name, variation := range read.variations {
		variations++

		if len(variation.Patterns) == 0 {
			t.Errorf("%s has no pattern to colour with", name)
		}

		if len(variation.Palettes) == 0 {
			t.Errorf("%s has no colour palette", name)
		}

		for number, choice := range variation.Patterns {
			for channel := range Channels {
				layer := choice.Layer(channel)

				if layer.Textures == "" {
					continue
				}

				textures, ok := read.Textures(layer.Textures)
				if !ok {
					t.Errorf("%s pattern %d channel %d names the pattern %q, which is in no file", name, number, channel, layer.Textures)

					continue
				}

				patterns++

				if textures.ColourMask == "" {
					t.Errorf("the pattern %q has no colour mask", layer.Textures)
				}

				if layer.Layout == "" {
					continue
				}

				if _, ok := read.Layout(layer.Layout); !ok {
					t.Errorf("%s pattern %d channel %d names the layout %q, which is in no file", name, number, channel, layer.Layout)

					continue
				}

				layouts++
			}
		}
	}

	t.Logf("read %d variations, %d patterns and %d layers with a layout", variations, patterns, layouts)
}
