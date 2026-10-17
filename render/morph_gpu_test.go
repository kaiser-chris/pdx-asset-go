//go:build uitest

package render

import (
	"fmt"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// narrowed gives the parts of a model built of one quad of two units across a
// blend shape that narrows it to one, which the attribute narrow drives.
func narrowed(t *testing.T, built *model.Model) *model.Model {
	t.Helper()

	file, err := mesh.Read(meshtest.New().Object(1, "object").Object(2, "s").Mesh(meshtest.Quad(2, 2), "").Bytes())
	if err != nil {
		t.Fatal(err)
	}

	shape, err := mesh.Read(meshtest.New().Object(1, "object").Object(2, "s").Mesh(meshtest.Quad(1, 2), "").Bytes())
	if err != nil {
		t.Fatal(err)
	}

	for index := range built.Parts {
		part := &built.Parts[index]
		source := file.Shapes[0].Meshes[0]
		source.Skin = nil

		converted := model.Convert(&source, model.Shape{ID: "bs_narrow", Mesh: &shape.Shapes[0].Meshes[0]})

		// The skin, if the part has one, is carried over as it was.
		for piece := range part.Pieces {
			part.Pieces[piece].BlendShapes = converted[piece].BlendShapes
		}

		part.Attributes = []model.Attribute{{Name: "narrow", BlendShape: "bs_narrow"}}
	}

	return built
}

// morphView draws a model from the front, framed at its unmorphed size.
func morphView(t *testing.T, renderer *Renderer, uploaded *Model) func() int {
	t.Helper()

	viewer := renderer.NewViewer(64, 64)
	t.Cleanup(viewer.Unload)

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Look.PaletteColor = [3]float32{1, 1, 1}

	return func() int {
		rl.BeginDrawing()
		viewer.Draw(uploaded)
		rl.EndDrawing()

		return covered(viewer.Image())
	}
}

// Morphing a model by its attributes changes what is drawn, part of the way
// for a weight below one, and a nil map puts it back.
func TestMorphChangesWhatIsDrawn(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(narrowed(t, quadModel(t, meshtest.Quad(2, 2))))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	if names := uploaded.Attributes(); len(names) != 1 || names[0] != "narrow" {
		t.Errorf("attributes = %v", names)
	}

	draw := morphView(t, renderer, uploaded)

	whole := draw()
	if whole == 0 {
		t.Fatal("nothing drawn of the quad")
	}

	uploaded.Morph(map[string]float64{"narrow": 1})
	narrow := draw()

	uploaded.Morph(map[string]float64{"narrow": 0.5, "unrelated": 1})
	between := draw()

	// Half as wide, and three quarters as wide, give or take the pixels on
	// the edges.
	if narrow > whole*6/10 || narrow < whole*4/10 || between >= whole || between <= narrow {
		t.Errorf("drawn %d pixels whole, %d narrowed, %d halfway", whole, narrow, between)
	}

	uploaded.Morph(nil)
	if back := draw(); back != whole {
		t.Errorf("drawn %d pixels after morphing back, want %d", back, whole)
	}
}

// A skinned part is morphed before it is posed: morphing keeps the pose it
// is in, and posing keeps the morph.
func TestMorphSkinnedGeometry(t *testing.T) {
	renderer := withRenderer(t)
	animation := translationAnimation(t)

	uploaded, err := renderer.Upload(narrowed(t, skinnedQuad(t, meshtest.Quad(2, 2))))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	draw := morphView(t, renderer, uploaded)

	whole := draw()

	uploaded.Morph(map[string]float64{"narrow": 1})
	narrow := draw()

	if whole == 0 || narrow > whole*6/10 {
		t.Fatalf("drawn %d pixels whole, %d narrowed", whole, narrow)
	}

	// Posed out of the picture, then morphed back: still out of it.
	uploaded.Pose(animation, 0.1, false, []int{0})
	uploaded.Morph(nil)

	if count := draw(); count != 0 {
		t.Errorf("%d pixels drawn after morphing a posed quad, want it still posed away", count)
	}

	// Morphed again, and posed back: narrow where it started.
	uploaded.Morph(map[string]float64{"narrow": 1})
	uploaded.Pose(nil, 0, false, nil)

	if count := draw(); count != narrow {
		t.Errorf("%d pixels drawn after posing back, want the %d of the narrowed quad", count, narrow)
	}
}

// slideClip is an additive clip of three frames for the four bones of
// skinnedQuad: at its middle they rest at the origin, at its end they are a
// long way along x.
func slideClip(t *testing.T) *anim.Animation {
	t.Helper()

	var joints []jointSpec
	for bone := range 4 {
		joints = append(joints, jointSpec{name: fmt.Sprintf("bone%d", bone), changes: "t", rest: anim.Pose{Translation: [3]float32{-100, 0, 0}, Rotation: stillQuat, Scale: [3]float32{1, 1, 1}}})
	}

	var translations []float32
	for _, x := range []float32{-100, 0, 100} {
		for range 4 {
			translations = append(translations, x, 0, 0)
		}
	}

	animation, err := anim.Read(buildAnim(2, 3, joints, translations, nil, nil))
	if err != nil {
		t.Fatal(err)
	}

	return animation
}

// An attribute that drives an additive animation plays it over the pose: at
// its default the quad stays where it rests, at either end of the clip it
// leaves the picture, and it does so over an animation as well.
func TestMorphPlaysAdditiveAnimations(t *testing.T) {
	renderer := withRenderer(t)

	built := skinnedQuad(t, meshtest.Quad(2, 2))
	built.Parts[0].Additive = []model.Additive{{ID: "slide", Animation: slideClip(t)}}
	built.Parts[0].Attributes = []model.Attribute{{Name: "slide_far", Animation: "slide", Default: 0.5}}

	uploaded, err := renderer.Upload(built)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	if names := uploaded.Attributes(); len(names) != 1 || names[0] != "slide_far" {
		t.Errorf("attributes = %v", names)
	}

	draw := morphView(t, renderer, uploaded)

	rest := draw()
	if rest == 0 {
		t.Fatal("nothing drawn of the quad")
	}

	uploaded.Morph(nil)
	if count := draw(); count != rest {
		t.Errorf("%d pixels drawn at the default, want the %d at rest", count, rest)
	}

	for _, value := range []float64{0.5, -0.5} {
		uploaded.Morph(map[string]float64{"slide_far": value})
		if count := draw(); count != 0 {
			t.Errorf("%d pixels drawn with the attribute at %g, want the quad slid away", count, value)
		}
	}

	uploaded.Morph(map[string]float64{"slide_far": 0})
	if count := draw(); count != rest {
		t.Errorf("%d pixels drawn back at the default, want %d", count, rest)
	}

	// Over an animation that leaves the bones where they rest, the layer
	// still slides the quad away; taking the animation off keeps the layer.
	uploaded.Morph(map[string]float64{"slide_far": 0.5})
	uploaded.Pose(translationAnimation(t), 0, false, []int{0})

	if count := draw(); count != 0 {
		t.Errorf("%d pixels drawn when posed with the layer at its end", count)
	}

	uploaded.Pose(nil, 0, false, nil)

	if count := draw(); count != 0 {
		t.Errorf("%d pixels drawn after taking the animation off, want the layer kept", count)
	}
}
