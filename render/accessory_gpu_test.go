//go:build uitest

package render

import (
	"image"
	"image/color"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/pattern"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// flat is a texture of one colour. It is not the shape of any accessory
// texture, but it is enough to tell one colour from another.
func filled(width, height int, fill [4]byte) *texture.Image {
	data := make([]byte, 0, width*height*4)

	for range width * height {
		data = append(data, fill[:]...)
	}

	return &texture.Image{Width: width, Height: height, Format: texture.RGBA8, Levels: 1, Data: data}
}

// paletteOf is a palette four pixels wide for every channel of a mask, as the
// games write them: the colours of the four channels side by side, each four
// shades deep.
func paletteOf(colours [pattern.Channels][4]byte) *texture.Image {
	data := make([]byte, 0, pattern.Channels*model.PaletteColumns*4)

	for _, colour := range colours {
		for range model.PaletteColumns {
			data = append(data, colour[:]...)
		}
	}

	return &texture.Image{Width: pattern.Channels * model.PaletteColumns, Height: 1, Format: texture.RGBA8, Levels: 1, Data: data}
}

// The colours of the fixture palette: red for the first channel of the mask,
// then green, then blue, then a dim red, which is told from the first by being
// darker.
var fixturePalette = [pattern.Channels][4]byte{
	{255, 0, 0, 255},
	{0, 255, 0, 255},
	{0, 0, 255, 255},
	{64, 0, 0, 255},
}

// paletteColors turns a palette texture into the colours a model carries, which
// is what the renderer is given rather than the texture itself.
func paletteColors(colours [pattern.Channels][4]byte) [pattern.Channels][3]float32 {
	var read [pattern.Channels][3]float32

	for channel, colour := range colours {
		for axis := range 3 {
			read[channel][axis] = float32(colour[axis]) / 255
		}
	}

	return read
}

// brightness is how much light a pixel sends back, which is what tells one
// surface from another where the colours are the same.
func brightness(pixel color.RGBA) int {
	return int(pixel.R) + int(pixel.G) + int(pixel.B)
}

// accessoryQuad is a quad of an effect that lays a pattern, coloured by an
// accessory whose pattern is white and whose two palettes colour every channel
// differently, or white.
func accessoryQuad(t *testing.T, mask [4]byte) *model.Model {
	t.Helper()

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Entity = "belt_entity"
	source.Parts[0].Shader = "portrait_attachment_pattern"
	source.Parts[0].Textures.Diffuse = filled(1, 1, [4]byte{255, 255, 255, 255})

	// The surface a pattern brings: a normal map of nothing is flat, which
	// leaves the shading of the part as it is, and a properties map of
	// nothing says nothing about the material.
	flatNormal := [4]byte{0, 128, 0, 128}
	plainProperties := [4]byte{0, 0, 0, 255}

	layers := [pattern.Channels]model.AccessoryLayer{}
	for channel := range pattern.Channels {
		layers[channel] = model.AccessoryLayer{
			ColourMask: filled(1, 1, [4]byte{255, 255, 255, 255}),
			Normal:     filled(1, 1, flatNormal),
			Properties: filled(1, 1, plainProperties),
			Placement:  pattern.Placement{Scale: 1},
		}
	}

	dark := layers
	for channel := range pattern.Channels {
		dark[channel].ColourMask = filled(1, 1, [4]byte{0, 0, 0, 255})
	}

	source.Parts[0].Accessory = &model.Accessory{
		Mask:      filled(1, 1, mask),
		Variation: "fixture_variation",
		Patterns: []model.AccessoryPattern{
			{Description: "white", Layers: layers},
			{Description: "black", Layers: dark},
		},
		Palettes: []model.AccessoryPalette{
			{Description: "white.dds", Colours: paletteColors([pattern.Channels][4]byte{{255, 255, 255, 255}, {255, 255, 255, 255}, {255, 255, 255, 255}, {255, 255, 255, 255}}), Read: true},
			{Description: "fixture.dds", Colours: paletteColors(fixturePalette), Read: true},
		},
		Pattern: 0,
		Palette: 1,
	}

	return source
}

// drawUploaded draws a model that is already on the GPU.
func drawUploaded(t *testing.T, viewer *Viewer, uploaded *Model) *image.RGBA {
	t.Helper()

	rl.BeginDrawing()
	viewer.Draw(uploaded)
	rl.EndDrawing()

	return viewer.Image()
}

// A pattern brings the surface of the accessory with it: where it covers a
// pixel its own normal map is what the light falls on, rather than the map of
// the mesh.
//
// Which way a normal map leans is not known without knowing the tangent frame
// of the mesh, so what is drawn is compared against the flat map of the same
// part: one lean is lit and the other is not, and the flat map lies between
// the two.
func TestDrawsTheSurfaceOfAPattern(t *testing.T) {
	renderer := withRenderer(t)

	var mask [4]byte
	mask[0] = 255

	draw := func(alpha byte) int {
		source := accessoryQuad(t, mask)

		if alpha != 0 {
			for channel := range pattern.Channels {
				// A normal map holds x in green and y in alpha, the other way
				// up, so alpha either side of its middle leans either way.
				source.Parts[0].Accessory.Patterns[0].Layers[channel].Normal = filled(1, 1, [4]byte{0, 128, 0, alpha})
			}
		}

		return brightness(centre(drawLook(t, renderer, source, [3]float32{1, 1, 1})))
	}

	flat, one, other := draw(0), draw(64), draw(192)

	if !(one < flat && flat < other) && !(other < flat && flat < one) {
		t.Errorf("a surface leaning either way draws %d and %d against the flat %d, want the flat between them", one, other, flat)
	}
}

// A pattern brings the material of the accessory with it too: a pattern whose
// properties map makes the surface metal is drawn as metal, which sends back
// no colour of its own and so is darker than the plain surface of the mesh.
func TestDrawsTheMaterialOfAPattern(t *testing.T) {
	renderer := withRenderer(t)

	var mask [4]byte
	mask[0] = 255

	plain := centre(drawLook(t, renderer, accessoryQuad(t, mask), [3]float32{1, 1, 1}))

	metal := accessoryQuad(t, mask)
	for channel := range pattern.Channels {
		// A properties map holds the metalness in blue.
		metal.Parts[0].Accessory.Patterns[0].Layers[channel].Properties = filled(1, 1, [4]byte{0, 0, 255, 128})
	}

	drawn := centre(drawLook(t, renderer, metal, [3]float32{1, 1, 1}))

	if brightness(drawn) >= brightness(plain) {
		t.Errorf("a pattern that makes the surface metal draws %v, want it darker than the plain %v", drawn, plain)
	}
}

// A part whose entity's game data names a portrait accessory is drawn in the
// colours of its palette, read through the channels of its mask: every channel
// of the mask brings the colour the palette holds for it, and a channel the
// mask does not cover brings none.
func TestDrawsTheAccessoryChannels(t *testing.T) {
	renderer := withRenderer(t)

	drawn := map[int]string{0: "red", 1: "green", 2: "blue"}

	for channel, want := range drawn {
		var mask [4]byte
		mask[channel] = 255

		picture := drawLook(t, renderer, accessoryQuad(t, mask), [3]float32{1, 1, 1})
		if got := centre(picture); dominant(got) != want {
			t.Errorf("the mask of channel %d draws %v, want %s", channel, got, want)
		}
	}

	// The fourth channel draws a dim red, which is that channel's own colour
	// rather than that of the first.
	var fourth [4]byte
	fourth[3] = 255

	dim := centre(drawLook(t, renderer, accessoryQuad(t, fourth), [3]float32{1, 1, 1}))
	if dominant(dim) != "red" || dim.R > 200 {
		t.Errorf("the mask of the fourth channel draws %v, want a dim red", dim)
	}

	// A mask that covers nothing leaves the part as it is, which is how the
	// same quad is drawn without an accessory at all.
	plain := quadModel(t, meshtest.Quad(2, 2))
	plain.Parts[0].Shader = "portrait_attachment_pattern"
	plain.Parts[0].Textures.Diffuse = filled(1, 1, [4]byte{255, 255, 255, 255})

	untouched := centre(drawLook(t, renderer, plain, [3]float32{1, 1, 1}))

	if got := centre(drawLook(t, renderer, accessoryQuad(t, [4]byte{}), [3]float32{1, 1, 1})); got != untouched {
		t.Errorf("a mask of nothing draws %v, want the part itself, %v", got, untouched)
	}
}

// A pattern is a mask, not a colour: one that covers everything in a single
// channel of its own draws the colour of the palette, as the plain silk of
// most of the shipped patterns does, rather than the colour of that channel.
func TestDrawsTheColourOfThePaletteThroughAPattern(t *testing.T) {
	renderer := withRenderer(t)

	var mask [4]byte
	mask[1] = 255

	source := accessoryQuad(t, mask)

	for channel := range pattern.Channels {
		source.Parts[0].Accessory.Patterns[0].Layers[channel].ColourMask = filled(1, 1, [4]byte{255, 0, 0, 255})
	}

	if got := centre(drawLook(t, renderer, source, [3]float32{1, 1, 1})); dominant(got) != "green" {
		t.Errorf("a pattern covering everything in its red channel draws %v, want the green of the palette", got)
	}
}

// Where the effect of a part lays no pattern, the accessory its entity names
// is not drawn by the game either, and the part keeps its own colour.
func TestDoesNotDrawTheAccessoryOfAnotherEffect(t *testing.T) {
	renderer := withRenderer(t)

	var mask [4]byte
	mask[0] = 255

	source := accessoryQuad(t, mask)
	source.Parts[0].Shader = "standard"

	// The part is drawn a shade of white, near enough that no one channel of
	// it stands out, which is what tells it from the red the accessory would
	// have drawn it.
	if got := centre(drawLook(t, renderer, source, [3]float32{1, 1, 1})); got.R < 200 || got.G < 200 || got.B < 200 {
		t.Errorf("a part whose effect lays no pattern draws %v, want its own white", got)
	}
}

// Another pattern and another palette of the same variation can be drawn
// without uploading the model again: the accessory offers what it has, and
// refuses what it has not.
func TestChoosesAnotherAlternative(t *testing.T) {
	renderer := withRenderer(t)

	var mask [4]byte
	mask[0] = 255

	source := accessoryQuad(t, mask)

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)

	// The first palette colours the red channel white, the second red, and
	// the second pattern draws nothing of the palette at all.
	if got := centre(drawUploaded(t, viewer, uploaded)); dominant(got) != "red" {
		t.Errorf("the accessory draws %v, want the red of the second palette", got)
	}

	choices := uploaded.Accessories()
	if len(choices) != 1 {
		t.Fatalf("accessories = %+v, want the one of the part", choices)
	}

	if choices[0].Entity != "belt_entity" || choices[0].Variation != "fixture_variation" ||
		choices[0].Patterns != 2 || choices[0].Palettes != 2 || choices[0].Palette != 1 {
		t.Errorf("the accessory offers %+v", choices[0])
	}

	if !uploaded.ChooseAccessory("belt_entity", 0, 0, 0) {
		t.Fatal("the accessory could not be drawn with its first palette")
	}

	if got := centre(drawUploaded(t, viewer, uploaded)); got.R < 200 || got.G < 200 || got.B < 200 {
		t.Errorf("the first palette draws %v, want white", got)
	}

	if !uploaded.ChooseAccessory("belt_entity", 0, 1, 1) {
		t.Fatal("the second pattern could not be drawn")
	}

	if got := centre(drawUploaded(t, viewer, uploaded)); got.R < 200 || got.G < 200 || got.B < 200 {
		t.Errorf("a pattern that covers nothing draws %v, want the part itself", got)
	}

	// What is not there is refused rather than drawn as something else.
	for _, refused := range []struct {
		entity                       string
		attachment, pattern, palette int
	}{
		{"belt_entity", 0, 0, 2},
		{"belt_entity", 0, 2, 0},
		{"belt_entity", 7, 0, 0},
		{"nothing", 0, 0, 0},
	} {
		if uploaded.ChooseAccessory(refused.entity, refused.attachment, refused.pattern, refused.palette) {
			t.Errorf("the accessory was drawn with pattern %d and palette %d, which it has not", refused.pattern, refused.palette)
		}
	}
}
