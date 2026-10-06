package entity

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// animationFile writes a .anim file of one joint over the given frames, at
// the given rate, which is enough to be listed.
func animationFile(fps float32, frames int) []byte {
	writer := meshtest.New()

	writer.Object(1, "info").
		Floats("fps", fps).
		Ints("sa", int32(frames)).
		Ints("j", 1)

	writer.Object(2, "millShape:root").
		Strings("sa", "q").
		Floats("t", 0, 0, 0).
		Floats("q", 0, 0, 0, -1).
		Floats("s", 1)

	writer.Object(1, "samples")

	turns := make([]float32, 0, frames*4)
	for range frames {
		turns = append(turns, 0, 0, 0, 1)
	}

	writer.Floats("q", turns...)

	return writer.Bytes()
}

// millAsset defines a mill whose mesh plays two animations of its own and the
// animations of a set it imports, a vane that is attached to it and plays one,
// and a mesh that names an animation whose file is not there.
const millAsset = `
skeletal_animation_set = {
	name = "shared_turning"
	reference_skeleton = "mill_mesh"
	animation = { id = "shared_spin" type = "shared_spin.anim" }
}

pdxmesh = {
	name = "mill_mesh"
	file = "mill.mesh"
	animation = { id = "idle_animation" type = "mill_idle.anim" }
	animation = { id = "working_animation" type = "mill_working.anim" }
	import = { type = skeletal_animation_set name = "shared_turning" }
	meshsettings = { name = "quadShape" index = 0 texture_diffuse = "mill_diffuse.png" shader = "standard" }
}

pdxmesh = {
	name = "vane_mesh"
	file = "mill.mesh"
	animation = { id = "vane_spin" type = "vane_spin.anim" }
	meshsettings = { name = "quadShape" index = 0 texture_diffuse = "mill_diffuse.png" shader = "standard" }
}

pdxmesh = {
	name = "silent_mesh"
	file = "mill.mesh"
	animation = { id = "lost_animation" type = "not_there.anim" }
	meshsettings = { name = "quadShape" index = 0 texture_diffuse = "mill_diffuse.png" shader = "standard" }
}

entity = {
	name = "mill_entity"
	pdxmesh = "mill_mesh"
	locator = { name = "top" position = { 0 3 0 } }
	attach = { top = "vane_entity" }
}

entity = { name = "vane_entity" pdxmesh = "vane_mesh" }
entity = { name = "silent_entity" pdxmesh = "silent_mesh" }
entity = { name = "still_entity" pdxmesh = "still_mesh" }

pdxmesh = {
	name = "still_mesh"
	file = "mill.mesh"
	meshsettings = { name = "quadShape" index = 0 texture_diffuse = "mill_diffuse.png" shader = "standard" }
}
`

func millGame(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/mill/mill.asset":       []byte(millAsset),
		"gfx/models/mill/mill.mesh":        meshtest.QuadFile(2, 2),
		"gfx/models/mill/mill_diffuse.png": picture(t, red),

		// Two seconds of a clip made at fifteen frames a second.
		"gfx/models/mill/mill_idle.anim": animationFile(15.5, 31),
		// Four seconds at thirty.
		"gfx/models/mill/mill_working.anim": animationFile(30.25, 121),
		"gfx/models/mill/shared_spin.anim":  animationFile(30, 60),
		"gfx/models/mill/vane_spin.anim":    animationFile(10, 5),
	})

	return root
}

func millLoader(t *testing.T) *Loader {
	t.Helper()

	root := millGame(t)
	set := folders.Open([]folders.Source{{Name: "game", Path: root}})

	return NewLoader(set, asset.Load(set))
}

// The animations an entity's mesh can play are listed with the model, its own
// first and then those of the sets it imports, each said to be of the entity
// and the attachment whose mesh plays it.
func TestLoadListsTheAnimations(t *testing.T) {
	built, diagnostics, err := millLoader(t).Load("mill_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(diagnostics) != 0 {
		t.Errorf("unexpected diagnostics: %v", diagnostics)
	}

	// The mill's own three, then the vane's, which is attachment one.
	want := []model.Animation{
		{ID: "idle_animation", Entity: "mill_entity", Attachment: 0, Mesh: "mill_mesh", FPS: 15.5, Frames: 31, Joints: 1, Seconds: 2},
		{ID: "working_animation", Entity: "mill_entity", Attachment: 0, Mesh: "mill_mesh", FPS: 30.25, Frames: 121, Joints: 1, Seconds: 4},
		{ID: "shared_spin", Entity: "mill_entity", Attachment: 0, Mesh: "mill_mesh", FPS: 30, Frames: 60, Joints: 1, Seconds: 2},
		{ID: "vane_spin", Entity: "vane_entity", Attachment: 1, Mesh: "vane_mesh", FPS: 10, Frames: 5, Joints: 1, Seconds: 0.5},
	}

	if len(built.Animations) != len(want) {
		t.Fatalf("animations = %+v, want %d of them", built.Animations, len(want))
	}

	for index, animation := range built.Animations {
		expected := want[index]

		if animation.ID != expected.ID || animation.Entity != expected.Entity ||
			animation.Attachment != expected.Attachment || animation.Mesh != expected.Mesh ||
			animation.FPS != expected.FPS || animation.Frames != expected.Frames || animation.Joints != expected.Joints {
			t.Errorf("animation %d = %+v, want %+v", index, animation, want[index])
		}

		if animation.Seconds != expected.Seconds {
			t.Errorf("animation %s runs %gs, want %gs", animation.ID, animation.Seconds, expected.Seconds)
		}

		if !strings.HasPrefix(animation.File, "gfx/models/mill/") {
			t.Errorf("animation %s is at %q", animation.ID, animation.File)
		}
	}

	// The timeline reaches as far as the longest of them.
	if got := model.Longest(built.Animations); got != 4 {
		t.Errorf("the longest runs %gs, want 4", got)
	}

	if got := model.Longest(nil); got != 0 {
		t.Errorf("no animations are %gs long", got)
	}
}

// An entity whose mesh names no animation has none, and one whose animation
// file is not there is reported and left out, rather than failing to load.
func TestAnimationsThatAreNotThere(t *testing.T) {
	loader := millLoader(t)

	still, diagnostics, err := loader.Load("still_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(still.Animations) != 0 || len(diagnostics) != 0 {
		t.Errorf("a mesh that names no animation has %+v and said %v", still.Animations, diagnostics)
	}

	silent, diagnostics, err := loader.Load("silent_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(silent.Animations) != 0 {
		t.Errorf("an animation that is not there was listed: %+v", silent.Animations)
	}

	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "lost_animation") ||
		!strings.Contains(diagnostics[0].Message, "in none of the folders") {
		t.Errorf("diagnostics = %v, want one about the animation that is not there", diagnostics)
	}

	// The model still draws.
	if len(silent.Parts) != 1 {
		t.Errorf("parts = %d, want the mesh to be drawn anyway", len(silent.Parts))
	}
}
