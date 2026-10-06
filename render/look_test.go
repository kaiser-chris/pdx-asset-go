package render

import (
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/model"
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
		{"tree_colormap", "", partLook{cutout: true, noMetal: true}},
		{"tree_two_sided", "", partLook{cutout: true, noMetal: true, twoSided: true}},
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
