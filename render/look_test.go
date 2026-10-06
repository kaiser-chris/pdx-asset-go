package render

import (
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
		{"portrait_hair", "", partLook{cutout: true, twoSided: true}},
		{"portrait_hair_alpha", "", partLook{cutout: true, twoSided: true}},
		{"standard_alpha_to_coverage", "", partLook{cutout: true}},
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
	if got, want := StyleOf(&hair), (Style{Cutout: true, TwoSided: true}); got != want {
		t.Errorf("hair: style = %+v, want %+v", got, want)
	}

	skin := model.Part{Shader: "portrait_skin_atlas"}
	if got, want := StyleOf(&skin), (Style{Palette: true, Atlas: true}); got != want {
		t.Errorf("skin: style = %+v, want %+v", got, want)
	}
}
