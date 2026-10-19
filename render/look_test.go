package render

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// The shader tells how to draw a part from the name of the effect the games
// would draw it with, as the three games name them.
func TestLook(t *testing.T) {
	for _, test := range []struct {
		shader  string
		subpass string
		want    partLook
	}{
		{"standard", "", partLook{}},
		{"portrait_skin", "", partLook{palette: true}},
		{"portrait_skin_face", "", partLook{palette: true}},
		{"portrait_hair", "", partLook{palette: true, paletteKind: paletteHair, cutout: true, twoSided: true}},
		{"portrait_hair_alpha", "", partLook{palette: true, paletteKind: paletteHair, cutout: true, twoSided: true, shadowless: true}},
		{"portrait_hair_double_sided", "", partLook{palette: true, paletteKind: paletteHairWhole, cutout: true, twoSided: true, shadowless: true}},
		{"portrait_eye", "", partLook{palette: true, paletteKind: paletteEye, shadowless: true}},
		{"standard_alpha_to_coverage", "", partLook{cutout: true}},
		// Glass and lace are laid over what is behind them by their alpha.
		{"portrait_attachment_alpha_to_coverage", "", partLook{coverage: true}},
		{"portrait_attachment_pattern_alpha_to_coverage", "", partLook{coverage: true, patterned: true}},
		{"tree_colormap", "", partLook{cutout: true, noMetal: true, foliage: true}},
		{"tree_two_sided", "", partLook{cutout: true, noMetal: true, twoSided: true, foliage: true}},
		{"decal_world", "Decals", partLook{blend: true}},
		{"decal_local", "", partLook{blend: true}},
		{"standard_alpha_blend", "", partLook{blend: true}},
		{"standard_two_sided", "", partLook{twoSided: true}},
		{"standard_atlas", "", partLook{atlas: true}},
		{"snap_to_terrain_atlas_usercolor", "", partLook{atlas: true}},
		// A part in a pass of decals is one whatever its effect is named.
		{"standard", "LocalDecals", partLook{blend: true}},
		// A part of no effect, of a bare mesh file.
		{"", "", partLook{palette: true}},
	} {
		part := model.Part{Shader: test.shader, Subpass: test.subpass}
		if got := lookOf(&part); got != test.want {
			t.Errorf("%s in %q: look = %+v, want %+v", test.shader, test.subpass, got, test.want)
		}
	}
}

// A tree whose files name a tint is coloured by it; one without falls back
// to the green of leaves.
func TestLookTintsTrees(t *testing.T) {
	tint := &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: []byte{0, 255, 0, 255}}

	tinted := model.Part{Shader: "tree_colormap", Textures: model.Textures{Tint: tint}}
	if got := lookOf(&tinted); !got.tinted || got.foliage {
		t.Errorf("tree with a tint: look = %+v, want tinted, no fallback", got)
	}

	// A tint is a tree's: a building is drawn as it is.
	building := model.Part{Shader: "standard", Textures: model.Textures{Tint: tint}}
	if got := lookOf(&building); got.foliage {
		t.Errorf("building: look = %+v, want no fallback green", got)
	}
}

// An application sees how a part is drawn the way the shader draws it.
func TestStyleOf(t *testing.T) {
	decal := model.Part{Shader: "standard", Subpass: "Decals"}
	if got, want := StyleOf(&decal), (Style{Blend: true}); got != want {
		t.Errorf("decal: style = %+v, want %+v", got, want)
	}

	hair := model.Part{Shader: "portrait_hair"}
	if got, want := StyleOf(&hair), (Style{Palette: true, Cutout: true, TwoSided: true}); got != want {
		t.Errorf("hair: style = %+v, want %+v", got, want)
	}

	skin := model.Part{Shader: "portrait_skin_atlas"}
	if got, want := StyleOf(&skin), (Style{Palette: true, Atlas: true}); got != want {
		t.Errorf("skin: style = %+v, want %+v", got, want)
	}
}

// A palette colour multiplies as the games multiply it in linear light: its
// ends stay where they are, and what is between is lighter than stored.
func TestPaletteColour(t *testing.T) {
	colour := PaletteColour([4]byte{0, 255, 128, 255})

	if colour[0] != 0 || colour[1] != 1 || colour[2] < 0.72 || colour[2] > 0.74 {
		t.Errorf("palette colour = %v, want 0, 1 and about 0.73", colour)
	}
}

// A colour of a palette read as sRGB multiplies in linear light by its own
// linear value, which the shader, taking a Look's colour to the power of the
// gamma, gets from the colour as it is stored; a palette read as it is stored
// multiplies by the stored value instead, so its colour is the brighter.
func TestSRGBPaletteColour(t *testing.T) {
	stored := [4]byte{0, 255, 128, 255}
	colour := SRGBPaletteColour(stored)

	if colour[0] != 0 || colour[1] != 1 || colour[2] < 0.50 || colour[2] > 0.51 {
		t.Errorf("palette colour = %v, want 0, 1 and about 0.5", colour)
	}

	if linear := math.Pow(float64(colour[2]), gamma); linear > 0.23 || linear < 0.21 {
		t.Errorf("multiplies by %v, want the linear value of 128, about 0.22", linear)
	}

	if PaletteColour(stored)[2] <= colour[2] {
		t.Error("a palette read as it is stored is no brighter than one read as sRGB")
	}
}
