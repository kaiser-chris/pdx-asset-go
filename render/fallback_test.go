package render

import (
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/model"
)

// The renderer's own shader tells how to draw a part from the name of the
// effect the games would draw it with, as the three games name them.
func TestFallbackLook(t *testing.T) {
	for _, test := range []struct {
		shader  string
		subpass string
		want    fallbackLook
	}{
		{"standard", "", fallbackLook{}},
		{"portrait_skin", "", fallbackLook{palette: true}},
		{"portrait_skin_face", "", fallbackLook{palette: true}},
		{"portrait_hair", "", fallbackLook{cutout: true, twoSided: true}},
		{"portrait_hair_alpha", "", fallbackLook{cutout: true, twoSided: true}},
		{"standard_alpha_to_coverage", "", fallbackLook{cutout: true}},
		{"tree_colormap", "", fallbackLook{cutout: true, noMetal: true}},
		{"tree_two_sided", "", fallbackLook{cutout: true, noMetal: true, twoSided: true}},
		{"decal_world", "Decals", fallbackLook{blend: true}},
		{"decal_local", "", fallbackLook{blend: true}},
		{"standard_alpha_blend", "", fallbackLook{blend: true}},
		{"standard_two_sided", "", fallbackLook{twoSided: true}},
		{"standard_atlas", "", fallbackLook{atlas: true}},
		{"snap_to_terrain_atlas_usercolor", "", fallbackLook{atlas: true}},
		// A part in a pass of decals is one whatever its effect is named.
		{"standard", "LocalDecals", fallbackLook{blend: true}},
		// A part of no effect, of a bare mesh file.
		{"", "", fallbackLook{palette: true}},
	} {
		part := model.Part{Shader: test.shader, Subpass: test.subpass}
		if got := fallbackLookOf(&part); got != test.want {
			t.Errorf("%s in %q: look = %+v, want %+v", test.shader, test.subpass, got, test.want)
		}
	}
}
