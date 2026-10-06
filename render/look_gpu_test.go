//go:build uitest

package render

import (
	"image"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// halvesOf is a texture of two pixels, left and right, of four bytes each.
func halvesOf(left, right [4]byte) *texture.Image {
	return &texture.Image{Width: 2, Height: 1, Format: texture.RGBA8, Levels: 1, Data: append(left[:], right[:]...)}
}

// drawLook draws a model with a palette colour.
func drawLook(t *testing.T, renderer *Renderer, source *model.Model, palette [3]float32) *image.RGBA {
	t.Helper()

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Look.PaletteColor = palette

	rl.BeginDrawing()
	viewer.Draw(uploaded)
	rl.EndDrawing()

	return viewer.Image()
}

// lookQuad is a quad of an effect of a name, with a diffuse map.
func lookQuad(t *testing.T, shader string, diffuse *texture.Image) *model.Model {
	t.Helper()

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Shader = shader
	source.Parts[0].Textures.Diffuse = diffuse

	return source
}

// What the diffuse map's alpha leaves out of leaves and hair is cut away.
func TestLookCutsOut(t *testing.T) {
	renderer := withRenderer(t)

	leaves := halvesOf([4]byte{0, 255, 0, 0}, [4]byte{0, 255, 0, 255})

	picture := drawLook(t, renderer, lookQuad(t, "tree_colormap", leaves), [3]float32{1, 1, 1})
	if left, right := picture.RGBAAt(20, 32), picture.RGBAAt(44, 32); left.A != 0 || right.A != 255 || dominant(right) != "green" {
		t.Errorf("leaves = %v then %v, want cut away then green", left, right)
	}

	// The same texture on a solid part is drawn whole.
	picture = drawLook(t, renderer, lookQuad(t, "standard", leaves), [3]float32{1, 1, 1})
	if left := picture.RGBAAt(20, 32); left.A != 255 {
		t.Errorf("solid part = %v, want drawn", left)
	}
}

// The grey leaves of a tree take the colour of its tint, or without one a
// green of leaves; what has a colour of its own, such as bark, keeps it.
func TestLookTintsLeaves(t *testing.T) {
	renderer := withRenderer(t)

	grey := halvesOf([4]byte{160, 160, 160, 255}, [4]byte{160, 160, 160, 255})
	blue := halvesOf([4]byte{40, 40, 200, 255}, [4]byte{40, 40, 200, 255})

	tinted := lookQuad(t, "tree_colormap", grey)
	tinted.Parts[0].Textures.Tint = blue
	if got := centre(drawLook(t, renderer, tinted, [3]float32{1, 1, 1})); dominant(got) != "blue" {
		t.Errorf("leaves of a blue tint = %v, want blue", got)
	}

	if got := centre(drawLook(t, renderer, lookQuad(t, "tree_colormap", grey), [3]float32{1, 1, 1})); !(got.G >= got.R && got.G > got.B+20) {
		t.Errorf("leaves of no tint = %v, want a green of leaves", got)
	}

	if got := centre(drawLook(t, renderer, lookQuad(t, "standard", grey), [3]float32{1, 1, 1})); got.G > got.B+20 {
		t.Errorf("a grey building = %v, want it left grey", got)
	}

	bark := halvesOf([4]byte{200, 40, 40, 255}, [4]byte{200, 40, 40, 255})
	if got := centre(drawLook(t, renderer, lookQuad(t, "tree", bark), [3]float32{1, 1, 1})); dominant(got) != "red" {
		t.Errorf("red bark of no tint = %v, want it kept red", got)
	}
}

// A decal is laid over the ground by its alpha, whichever comes first.
func TestLookBlendsDecals(t *testing.T) {
	renderer := withRenderer(t)

	red := halvesOf([4]byte{255, 0, 0, 255}, [4]byte{255, 0, 0, 255})
	halfBlue := halvesOf([4]byte{0, 0, 255, 128}, [4]byte{0, 0, 255, 128})

	decal := lookQuad(t, "decal_local", halfBlue).Parts[0]
	decal.Subpass = "LocalDecals"
	ground := lookQuad(t, "standard", red).Parts[0]

	source := &model.Model{Name: "decal", Parts: []model.Part{decal, ground}}
	source.Bounds()

	got := centre(drawLook(t, renderer, source, [3]float32{1, 1, 1}))
	if got.A != 255 || got.R < 20 || got.B < 20 {
		t.Errorf("decal over the ground = %v, want red and blue mixed, opaque", got)
	}
}

// The palette colour is blended into skin only, not into a building whose
// diffuse map's alpha means something else.
func TestLookPaletteOnSkin(t *testing.T) {
	renderer := withRenderer(t)

	white := halvesOf([4]byte{255, 255, 255, 255}, [4]byte{255, 255, 255, 255})
	green := [3]float32{0.2, 1, 0.2}

	if got := centre(drawLook(t, renderer, lookQuad(t, "portrait_skin", white), green)); dominant(got) != "green" {
		t.Errorf("skin = %v, want the palette's green", got)
	}

	if got := centre(drawLook(t, renderer, lookQuad(t, "standard", white), green)); dominant(got) == "green" && got.G > got.R+20 {
		t.Errorf("building = %v, want no palette colour", got)
	}
}

// The normal map is read as the games' UnpackRRxGNormal reads it: x in
// green, y in alpha, red unused. Read the other way round, the halves of a
// face whose texture is mirrored are lit differently, a seam down its
// middle.
func TestLookReadsNormalsAsTheGames(t *testing.T) {
	renderer := withRenderer(t)

	grey := halvesOf([4]byte{200, 200, 200, 255}, [4]byte{200, 200, 200, 255})

	shade := func(normal [4]byte) uint8 {
		source := lookQuad(t, "standard", grey)
		source.Parts[0].Textures.Normal = halvesOf(normal, normal)

		return centre(drawLook(t, renderer, source, [3]float32{1, 1, 1})).R
	}

	flat := shade(flatNormal)

	if got := shade([4]byte{0, 128, 255, 128}); got != flat {
		t.Errorf("red changed the shading: %d, flat %d", got, flat)
	}

	if got := shade([4]byte{128, 128, 255, 255}); near(got, int(flat)) {
		t.Errorf("alpha left the shading as it is: %d, flat %d", got, flat)
	}

	if got := shade([4]byte{128, 255, 255, 128}); near(got, int(flat)) {
		t.Errorf("green left the shading as it is: %d, flat %d", got, flat)
	}

	// No normal map at all is the flat one.
	source := lookQuad(t, "standard", grey)
	if got := centre(drawLook(t, renderer, source, [3]float32{1, 1, 1})).R; got != flat {
		t.Errorf("without a normal map: %d, want the flat %d", got, flat)
	}
}

// A part of an atlas reads its textures by the second set of texture
// coordinates, as the games' effects of ATLAS do.
func TestLookReadsAtlasesByTheSecondCoordinates(t *testing.T) {
	renderer := withRenderer(t)

	halves := halvesOf([4]byte{255, 0, 0, 255}, [4]byte{0, 0, 255, 255})

	// The quad's second coordinates are all on the left, red half.
	atlas := lookQuad(t, "standard_atlas", halves)
	for index := range atlas.Parts[0].Pieces {
		uv1 := atlas.Parts[0].Pieces[index].UV1
		for at := range uv1 {
			uv1[at] = 0.1
		}
	}

	if got := drawLook(t, renderer, atlas, [3]float32{1, 1, 1}).RGBAAt(44, 32); dominant(got) != "red" {
		t.Errorf("right of the atlas part = %v, want the red the second coordinates point at", got)
	}

	plain := lookQuad(t, "standard", halves)
	if got := drawLook(t, renderer, plain, [3]float32{1, 1, 1}).RGBAAt(44, 32); dominant(got) != "blue" {
		t.Errorf("right of a plain part = %v, want blue", got)
	}
}
