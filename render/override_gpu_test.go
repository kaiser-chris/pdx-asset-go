//go:build uitest

package render

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// A texture override draws the skin of its body part with its maps in the
// place of the part's own, decals and all: a dark face is drawn dark, a
// decal laid before the override is laid again over the new map, and an
// override of another body part, or of no maps, leaves the part as it was.
func TestOverrideSkinDrawsTheSkinWithOtherMaps(t *testing.T) {
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
	plain := brightness(drawUploaded(t, viewer, uploaded).RGBAAt(46, 32))

	darker := [4]byte{90, 90, 90, 0}
	override := SkinMaps{Diffuse: row(darker, darker, darker, darker)}

	if err := uploaded.OverrideSkin("torso", override); err != nil {
		t.Fatal(err)
	}

	if got := uploaded.parts[0].material.GetMap(rl.MapDiffuse).Texture.ID; got != own {
		t.Errorf("overriding the torso draws the head with texture %d, want its own %d", got, own)
	}

	if err := uploaded.OverrideSkin("head", override); err != nil {
		t.Fatal(err)
	}

	overridden := brightness(drawUploaded(t, viewer, uploaded).RGBAAt(46, 32))
	if overridden >= plain*3/4 {
		t.Errorf("the overridden head is drawn %d, %d with its own map; want it darker", overridden, plain)
	}

	// A decal that darkens the left half is laid over the override, and stays
	// laid over the part's own map once the override is taken away.
	dark := [4]byte{40, 40, 40, 255}
	none := [4]byte{40, 40, 40, 0}

	err = uploaded.LayDecals([]Decal{{BodyPart: "head", Diffuse: row(dark, dark, none, none), DiffuseMode: BlendMultiply, Weight: 1}}, DefaultLook)
	if err != nil {
		t.Fatal(err)
	}

	decaled := drawUploaded(t, viewer, uploaded)
	if left, right := brightness(decaled.RGBAAt(18, 32)), brightness(decaled.RGBAAt(46, 32)); left >= right/2 || math.Abs(float64(right-overridden)) > 6 {
		t.Errorf("with a decal over the override the halves are %d and %d; want the left much darker and the right %d", left, right, overridden)
	}

	if err := uploaded.OverrideSkin("head", SkinMaps{}); err != nil {
		t.Fatal(err)
	}

	restored := drawUploaded(t, viewer, uploaded)
	if left, right := brightness(restored.RGBAAt(18, 32)), brightness(restored.RGBAAt(46, 32)); left >= right/2 || math.Abs(float64(right-plain)) > 6 {
		t.Errorf("with the override taken away the halves are %d and %d; want the left much darker and the right %d", left, right, plain)
	}

	if err := uploaded.LayDecals(nil, DefaultLook); err != nil {
		t.Fatal(err)
	}

	if got := uploaded.parts[0].material.GetMap(rl.MapDiffuse).Texture.ID; got != own {
		t.Errorf("with no override and no decals the head is drawn with texture %d, want its own %d", got, own)
	}
}
