//go:build uitest

package render

import (
	"image/color"
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// row is a texture one pixel high, of the pixels given, left to right.
func row(pixels ...[4]byte) *texture.Image {
	data := make([]byte, 0, len(pixels)*4)

	for _, pixel := range pixels {
		data = append(data, pixel[:]...)
	}

	return &texture.Image{Width: len(pixels), Height: 1, Format: texture.RGBA8, Levels: 1, Data: data}
}

// skinQuad is a quad of skin that is the head of a portrait, whose diffuse
// map is four pixels of one colour.
func skinQuad(t *testing.T, diffuse [4]byte) *model.Model {
	t.Helper()

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Shader = "portrait_skin_face"
	source.Parts[0].DecalBodyPart = "head"
	source.Parts[0].Textures.Diffuse = row(diffuse, diffuse, diffuse, diffuse)

	return source
}

// laidMap reads a map of the uploaded quad back from the GPU, as the quad is
// drawn with it.
func laidMap(t *testing.T, uploaded *Model, slot int32) []color.RGBA {
	t.Helper()

	image := rl.LoadImageFromTexture(uploaded.parts[0].material.GetMap(slot).Texture)
	if image == nil {
		t.Fatal("the map could not be read back")
	}
	defer rl.UnloadImage(image)

	return rl.LoadImageColors(image)
}

// The blending of the games' portrait_decals.fxh and
// texture_decals_base.fxh, written out again on the CPU to check the shader
// against: one channel, in linear light for a colour.
func overlayDecal(target, blend float64) float64 {
	if target > 0.5 {
		return 1 - 2*(1-target)*(1-blend)
	}

	return 2 * target * blend
}

func hardLightDecal(target, blend float64) float64 {
	if blend > 0.5 {
		return 1 - 2*(1-target)*(1-blend)
	}

	return 2 * target * blend
}

func blendDecal(mode BlendMode, target, blend, weight float64) float64 {
	result := 0.0

	switch mode {
	case BlendOverlay:
		result = overlayDecal(target, blend)
	case BlendReplace:
		result = blend
	case BlendHardLight:
		result = hardLightDecal(target, blend)
	case BlendMultiply:
		result = target * blend
	}

	return target + (result-target)*weight
}

func toLinear(value byte) float64 {
	return math.Pow(float64(value)/255, gamma)
}

func fromLinear(value float64) float64 {
	return math.Pow(min(max(value, 0), 1), 1/gamma) * 255
}

// nearByte reports whether a channel read back is within two steps of what it
// should be, for the rounding of half floats and of the bytes of the map.
func nearByte(got uint8, want float64) bool {
	return math.Abs(float64(got)-want) <= 2
}

// Each blend mode lays a decal's colour over the skin the way the games'
// BlendDecal does, in linear light, as opaque as the decal's weight and the
// alpha of its diffuse map make it; the colour mask in the skin's alpha is
// left as it was.
func TestLaysTheColourOfADecalTheWayTheGameDoes(t *testing.T) {
	renderer := withRenderer(t)

	skin := [4]byte{200, 150, 60, 77}
	decal := [4]byte{128, 64, 230, 255}
	halfOpaque := [4]byte{128, 64, 230, 128}

	for _, test := range []struct {
		name   string
		mode   BlendMode
		pixel  [4]byte
		weight float64
	}{
		{"overlay", BlendOverlay, decal, 0.5},
		{"replace", BlendReplace, decal, 0.75},
		{"hard_light", BlendHardLight, decal, 1},
		{"multiply", BlendMultiply, decal, 0.5},
		{"multiply through half an alpha", BlendMultiply, halfOpaque, 1},
	} {
		uploaded, err := renderer.Upload(skinQuad(t, skin))
		if err != nil {
			t.Fatal(err)
		}

		err = uploaded.LayDecals([]Decal{{
			BodyPart:    "head",
			Diffuse:     row(test.pixel, test.pixel, test.pixel, test.pixel),
			DiffuseMode: test.mode,
			Weight:      test.weight,
		}}, DefaultLook)
		if err != nil {
			t.Fatal(err)
		}

		got := laidMap(t, uploaded, rl.MapDiffuse)[1]
		uploaded.Unload()

		weight := test.weight * float64(test.pixel[3]) / 255

		want := [4]float64{
			fromLinear(blendDecal(test.mode, toLinear(skin[0]), toLinear(test.pixel[0]), weight)),
			fromLinear(blendDecal(test.mode, toLinear(skin[1]), toLinear(test.pixel[1]), weight)),
			fromLinear(blendDecal(test.mode, toLinear(skin[2]), toLinear(test.pixel[2]), weight)),
			float64(skin[3]),
		}

		if !nearByte(got.R, want[0]) || !nearByte(got.G, want[1]) || !nearByte(got.B, want[2]) || !nearByte(got.A, want[3]) {
			t.Errorf("%s: laid %v, want %.1f", test.name, got, want)
		}
	}
}

// A decal's normal map is laid over the skin's as the games' overlay of a
// normal does: the slopes of the two added, the heights multiplied, and the
// sum normalised, packed back into green and alpha like the skin's own. The
// red and blue of the skin's normal map are left as they were.
func TestLaysTheNormalOfADecalTheWayTheGameDoes(t *testing.T) {
	renderer := withRenderer(t)

	source := skinQuad(t, [4]byte{200, 200, 200, 0})
	skinNormal := [4]byte{10, 160, 20, 100}
	source.Parts[0].Textures.Normal = row(skinNormal, skinNormal, skinNormal, skinNormal)

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	decalNormal := [4]byte{0, 210, 0, 150}

	err = uploaded.LayDecals([]Decal{{
		BodyPart:   "head",
		Normal:     row(decalNormal, decalNormal, decalNormal, decalNormal),
		NormalMode: BlendOverlay,
		Weight:     0.6,
	}}, DefaultLook)
	if err != nil {
		t.Fatal(err)
	}

	got := laidMap(t, uploaded, rl.MapNormal)[2]

	unpack := func(pixel [4]byte) (x, y float64) {
		return float64(pixel[1])/255*2 - 1, 1 - float64(pixel[3])/255*2
	}

	weight := 0.6

	sx, sy := unpack(skinNormal)
	sz := math.Sqrt(1 - sx*sx - sy*sy)

	dx, dy := unpack(decalNormal)
	dx, dy = dx*weight, dy*weight
	dz := math.Sqrt(1 - dx*dx - dy*dy)

	x := sx + (sx+dx-sx)*weight
	y := sy + (sy+dy-sy)*weight
	z := sz + (sz*dz-sz)*weight

	length := math.Sqrt(x*x + y*y + z*z)
	x, y = x/length, y/length

	if !nearByte(got.G, (x*0.5+0.5)*255) || !nearByte(got.A, (1-y)*0.5*255) || got.R != skinNormal[0] || got.B != skinNormal[2] {
		t.Errorf("laid %v, want green %.1f and alpha %.1f, with red and blue %d and %d", got, (x*0.5+0.5)*255, (1-y)*0.5*255, skinNormal[0], skinNormal[2])
	}
}

// Decals are laid in order of priority, whatever order they are given in: of
// two that replace the skin, the one of the higher priority is what is left.
// Of the same priority, the one given last is.
func TestLaysDecalsInOrderOfPriority(t *testing.T) {
	renderer := withRenderer(t)

	red := [4]byte{255, 0, 0, 255}
	blue := [4]byte{0, 0, 255, 255}

	replace := func(pixel [4]byte, priority int) Decal {
		return Decal{BodyPart: "head", Diffuse: row(pixel, pixel, pixel, pixel), DiffuseMode: BlendReplace, Weight: 1, Priority: priority}
	}

	for _, test := range []struct {
		name   string
		decals []Decal
		want   string
	}{
		{"by priority", []Decal{replace(blue, 2), replace(red, 1)}, "blue"},
		{"the other way", []Decal{replace(red, 2), replace(blue, 1)}, "red"},
		{"of the same priority", []Decal{replace(red, 0), replace(blue, 0)}, "blue"},
	} {
		uploaded, err := renderer.Upload(skinQuad(t, [4]byte{128, 128, 128, 255}))
		if err != nil {
			t.Fatal(err)
		}

		if err := uploaded.LayDecals(test.decals, DefaultLook); err != nil {
			t.Fatal(err)
		}

		if got := dominant(laidMap(t, uploaded, rl.MapDiffuse)[0]); got != test.want {
			t.Errorf("%s: %s is left, want %s", test.name, got, test.want)
		}

		uploaded.Unload()
	}
}

// A decal is laid only over the body part it names, and only over skin:
// the games lay decals in their skin shader alone. A part drawn with another
// effect, or that is another body part, keeps its own maps.
func TestLaysDecalsOverTheirOwnBodyPart(t *testing.T) {
	renderer := withRenderer(t)

	black := [4]byte{0, 0, 0, 255}
	decal := Decal{BodyPart: "head", Diffuse: row(black, black, black, black), DiffuseMode: BlendReplace, Weight: 1}

	for _, test := range []struct {
		name  string
		setup func(*model.Part)
		decal Decal
		laid  bool
	}{
		{"the head", func(*model.Part) {}, decal, true},
		{"the torso", func(part *model.Part) { part.DecalBodyPart = "torso" }, decal, false},
		{"no body part", func(part *model.Part) { part.DecalBodyPart = "" }, decal, false},
		{"not skin", func(part *model.Part) { part.Shader = "portrait_attachment" }, decal, false},
		// Europa Universalis 5 names its hair a part of the head too, which its
		// hair effect draws without decals, as its eyes are.
		{"hair of the head", func(part *model.Part) { part.Shader = "portrait_hair_alpha" }, decal, false},
		{"eyes of the head", func(part *model.Part) { part.Shader = "portrait_eye" }, decal, false},
	} {
		source := skinQuad(t, [4]byte{200, 200, 200, 0})
		test.setup(&source.Parts[0])

		uploaded, err := renderer.Upload(source)
		if err != nil {
			t.Fatal(err)
		}

		own := uploaded.parts[0].material.GetMap(rl.MapDiffuse).Texture.ID

		if err := uploaded.LayDecals([]Decal{test.decal}, DefaultLook); err != nil {
			t.Fatal(err)
		}

		pixel := laidMap(t, uploaded, rl.MapDiffuse)[0]
		laid := uploaded.parts[0].material.GetMap(rl.MapDiffuse).Texture.ID != own

		if laid != test.laid || test.laid && brightness(pixel) != 0 || !test.laid && pixel.R != 200 {
			t.Errorf("%s: laid %v, the map is %v; want laid %v", test.name, laid, pixel, test.laid)
		}

		uploaded.Unload()
	}
}

// A decal is drawn: one that darkens the left half of a quad's skin draws
// the left half darker than the quad without it, and leaves the right half as
// it was. Laying no decals at all draws the quad with its own map again.
func TestDrawsDecals(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(skinQuad(t, [4]byte{220, 220, 220, 0}))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)

	own := uploaded.parts[0].material.GetMap(rl.MapDiffuse).Texture.ID
	plain := drawUploaded(t, viewer, uploaded)

	dark := [4]byte{40, 40, 40, 255}
	none := [4]byte{40, 40, 40, 0}

	err = uploaded.LayDecals([]Decal{{
		BodyPart:    "head",
		Diffuse:     row(dark, dark, none, none),
		DiffuseMode: BlendMultiply,
		Weight:      1,
	}}, DefaultLook)
	if err != nil {
		t.Fatal(err)
	}

	decaled := drawUploaded(t, viewer, uploaded)

	left, right := brightness(decaled.RGBAAt(18, 32)), brightness(decaled.RGBAAt(46, 32))
	plainLeft, plainRight := brightness(plain.RGBAAt(18, 32)), brightness(plain.RGBAAt(46, 32))

	if left >= plainLeft/2 {
		t.Errorf("the left half is drawn %d with the decal, %d without; want it much darker", left, plainLeft)
	}

	if math.Abs(float64(right-plainRight)) > 6 {
		t.Errorf("the right half is drawn %d with the decal, %d without; want it as it was", right, plainRight)
	}

	if err := uploaded.LayDecals(nil, DefaultLook); err != nil {
		t.Fatal(err)
	}

	if got := uploaded.parts[0].material.GetMap(rl.MapDiffuse).Texture.ID; got != own {
		t.Errorf("with no decals the quad is drawn with texture %d, want its own %d", got, own)
	}

	if again := drawUploaded(t, viewer, uploaded); brightness(again.RGBAAt(18, 32)) != plainLeft {
		t.Errorf("with no decals the left half is drawn %d, want %d as before", brightness(again.RGBAAt(18, 32)), plainLeft)
	}
}

// A decal laid after the skin colour is not tinted by it, where one laid
// before is: the skin colour is blended into the map before the decal is
// laid, and the part is then drawn without blending it in again.
func TestLaysDecalsAfterTheSkinColour(t *testing.T) {
	renderer := withRenderer(t)

	green := [4]byte{0, 255, 0, 255}
	clear := [4]byte{0, 255, 0, 0}
	skin := [4]byte{200, 200, 200, 255}

	look := Look{PaletteColor: [3]float32{1, 0.5, 0.5}, ColorMaskInterval: [2]float32{0, 1}}

	for _, after := range []bool{false, true} {
		uploaded, err := renderer.Upload(skinQuad(t, skin))
		if err != nil {
			t.Fatal(err)
		}

		err = uploaded.LayDecals([]Decal{{
			BodyPart:      "head",
			Diffuse:       row(green, green, clear, clear),
			DiffuseMode:   BlendReplace,
			Weight:        1,
			PostSkinColor: after,
		}}, look)
		if err != nil {
			t.Fatal(err)
		}

		laid := laidMap(t, uploaded, rl.MapDiffuse)
		paletted := uploaded.parts[0].decals.paletted
		uploaded.Unload()

		if paletted != after {
			t.Errorf("laid after the skin colour %v: the map has the skin colour blended in %v", after, paletted)
		}

		if got := laid[0]; got.R > 2 || got.G < 253 || got.B > 2 {
			t.Errorf("laid after the skin colour %v: the decal is %v, want green", after, got)
		}

		// The skin colour, blended in by the games in linear light, which a
		// Look holds as sRGB.
		want := [3]float64{200, 200, 200}

		if after {
			for channel := range want {
				palette := math.Pow(float64(look.PaletteColor[channel]), gamma)
				want[channel] = fromLinear(toLinear(skin[channel]) * palette)
			}
		}

		if got := laid[3]; !nearByte(got.R, want[0]) || !nearByte(got.G, want[1]) || !nearByte(got.B, want[2]) {
			t.Errorf("laid after the skin colour %v: the skin is %v, want %.1f", after, got, want)
		}
	}
}

// Decals are laid with a look that lights with an environment as with one
// that does not: the look's lighting is no business of the decals.
func TestDecalsWithLighting(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(quadModel(t, meshtest.Quad(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	look := DefaultLook
	look.Lighting = &Lighting{}

	if err := uploaded.LayDecals(nil, look); err != nil {
		t.Fatal(err)
	}
}
