//go:build uitest

package render

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/entity"
)

// TestRenderInstalledMaleBody draws the male body of a real installation from
// the front, the side and the back, and checks that each view shows a figure.
//
// It is skipped unless PDX_GAME_DIR points at a game folder. Set PDX_DUMP_DIR
// as well to write the views out as PNG files, which is the way to see that
// the body is drawn the way the game draws it.
func TestRenderInstalledMaleBody(t *testing.T) {
	game := os.Getenv("PDX_GAME_DIR")
	if game == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	set := folders.Open([]folders.Source{{Name: "game", Path: game}})
	assets := asset.Load(set)

	if !assets.Entities.Has("male_body_entity") {
		t.Skip("the game has no male_body_entity")
	}

	built, _, err := entity.NewLoader(set, assets).Load("male_body_entity")
	if err != nil {
		t.Fatal(err)
	}

	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(built)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(512, 512)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)

	for _, view := range []struct {
		name string
		yaw  float64
	}{
		{"front", 0},
		{"side", math.Pi / 2},
		{"back", math.Pi},
		{"three_quarters", math.Pi / 4},
	} {
		viewer.Reset()
		viewer.Rotate(view.yaw, 0.15)

		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()

		picture := viewer.Image()

		// A figure standing in the middle of the picture: a good part of it
		// drawn, the corners left empty.
		count := covered(picture)
		if count < 512*512/20 || picture.RGBAAt(0, 0).A != 0 || picture.RGBAAt(511, 511).A != 0 {
			t.Errorf("%s: %d of %d pixels drawn", view.name, count, 512*512)
		}

		// Lit and textured, the body is not one flat colour.
		if view.name == "front" && flat(picture) {
			t.Errorf("%s: the body is drawn in one flat colour", view.name)
		}

		dump(t, picture, fmt.Sprintf("male_body_%s.png", view.name))
	}
}

// flat reports whether every pixel drawn has the same colour.
func flat(picture *image.RGBA) bool {
	var first []uint8

	for index := 0; index+3 < len(picture.Pix); index += 4 {
		if picture.Pix[index+3] == 0 {
			continue
		}

		if first == nil {
			first = picture.Pix[index : index+3]

			continue
		}

		if picture.Pix[index] != first[0] || picture.Pix[index+1] != first[1] || picture.Pix[index+2] != first[2] {
			return false
		}
	}

	return true
}

// dump writes a picture to PDX_DUMP_DIR, when it is set.
func dump(t *testing.T, picture *image.RGBA, name string) {
	t.Helper()

	folder := os.Getenv("PDX_DUMP_DIR")
	if folder == "" {
		return
	}

	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}

	file, err := os.Create(filepath.Join(folder, name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := png.Encode(file, picture); err != nil {
		t.Fatal(err)
	}
}

// TestRenderInstalledMorphedHead draws the male head of a real installation
// neutral and morphed by a few of the attributes its genes set, from
// the side, where the nose and the chin show, and checks that the morphs
// change what is drawn and that morphing back undoes them.
func TestRenderInstalledMorphedHead(t *testing.T) {
	game := os.Getenv("PDX_GAME_DIR")
	if game == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	set := folders.Open([]folders.Source{{Name: "game", Path: game}})
	assets := asset.Load(set)

	if !assets.Entities.Has("male_head_entity") {
		t.Skip("the game has no male_head_entity")
	}

	built, _, err := entity.NewLoader(set, assets).Load("male_head_entity")
	if err != nil {
		t.Fatal(err)
	}

	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(built)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(256, 256)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Rotate(math.Pi/2, 0)

	draw := func(name string) *image.RGBA {
		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()

		picture := viewer.Image()
		dump(t, picture, name)

		return picture
	}

	// Neutral, every attribute at its default: the additive animations at
	// their middles, which are not all quite where the mesh rests.
	uploaded.Morph(nil)
	stored := draw("male_head_stored.png")

	uploaded.Morph(map[string]float64{
		"head_bs_nose_size_max":   1,
		"head_bs_nose_length_max": 1,
		"head_bs_jaw_def_max":     1,
	})

	morphed := draw("male_head_morphed.png")

	if changed := differing(stored, morphed); changed < 50 {
		t.Errorf("morphing changed %d pixels, want the nose and jaw to show", changed)
	}

	uploaded.Morph(nil)

	if changed := differing(stored, draw("male_head_unmorphed.png")); changed != 0 {
		t.Errorf("morphing back left %d pixels changed", changed)
	}
}

// differing counts the pixels two pictures of the same size differ in.
func differing(a, b *image.RGBA) int {
	count := 0

	for index := 0; index+3 < len(a.Pix) && index+3 < len(b.Pix); index += 4 {
		if [4]uint8(a.Pix[index:index+4]) != [4]uint8(b.Pix[index:index+4]) {
			count++
		}
	}

	return count
}

// TestRenderInstalledChinForward draws the male head of a real installation
// from the side with its chin as far forward as the genes put it, which an
// additive animation does, and checks that it shows and goes back.
func TestRenderInstalledChinForward(t *testing.T) {
	game := os.Getenv("PDX_GAME_DIR")
	if game == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	set := folders.Open([]folders.Source{{Name: "game", Path: game}})
	assets := asset.Load(set)

	if !assets.Entities.Has("male_head_entity") {
		t.Skip("the game has no male_head_entity")
	}

	built, _, err := entity.NewLoader(set, assets).Load("male_head_entity")
	if err != nil {
		t.Fatal(err)
	}

	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(built)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(256, 256)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Rotate(math.Pi/2, 0)

	draw := func(name string) *image.RGBA {
		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()

		picture := viewer.Image()
		dump(t, picture, name)

		return picture
	}

	uploaded.Morph(nil)
	neutral := draw("male_head_chin_neutral.png")

	uploaded.Morph(map[string]float64{"head_chin_forward": 0.5, "head_jaw_width": 0.5})
	forward := draw("male_head_chin_forward.png")

	if changed := differing(neutral, forward); changed < 30 {
		t.Errorf("moving the chin forward changed %d pixels", changed)
	}

	uploaded.Morph(nil)

	if changed := differing(neutral, draw("male_head_chin_back.png")); changed != 0 {
		t.Errorf("moving the chin back left %d pixels changed", changed)
	}
}
