package entity

import (
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// townAsset attaches blocks, quads of two by two, to entities in every way
// the shipped files do.
const townAsset = `
pdxmesh = {
	name = "block_mesh"
	file = "block.mesh"
	meshsettings = { name = "quadShape" index = 0 texture_diffuse = "block_diffuse.png" shader = "standard" }
}
entity = { name = "block_entity" pdxmesh = "block_mesh" }
entity = { name = "big_block_entity" clone = "block_entity" scale = 2 }

# A place of nothing but what it attaches.
entity = {
	name = "square_entity"
	locator = { name = "east" position = { 10 0 0 } }
	locator = { name = "turned" position = { 0 0 10 } rotation = { 0 90 0 } }
	attach = { east = "big_block_entity" }
	attach = { turned = "block_entity" }
}

# A rig: a locator of its mesh file, one that hangs from a bone, and the bone.
pdxmesh = { name = "rig_mesh" file = "rig.mesh" meshsettings = { name = "quadShape" index = 0 shader = "standard" } }
entity = {
	name = "rig_entity"
	pdxmesh = "rig_mesh"
	attach = { top = "block_entity" hand = "block_entity" }
	attach = { SPINE = "block_entity" }
}

# Attachments to attachments.
entity = {
	name = "district_entity"
	locator = { name = "north" position = { 0 0 100 } }
	attach = { north = "square_entity" }
}

# A rider that a dragoon clones, with his weapon swapped.
entity = {
	name = "rider_entity"
	pdxmesh = "block_mesh"
	locator = { name = "hand" position = { 1 0 0 } }
	locator = { name = "back" position = { 0 0 -1 } }
	attach = { name = weapon hand = "sabre_entity" }
	attach = { back = "pack_entity" }
}
entity = { name = "dragoon_entity" clone = "rider_entity" attach = { name = weapon hand = "rifle_entity" } }
entity = { name = "sabre_entity" clone = "block_entity" }
entity = { name = "rifle_entity" clone = "block_entity" }
entity = { name = "pack_entity" clone = "block_entity" }

# Scaffolding the engine picks at random.
entity = {
	name = "site_entity"
	locator = { name = "here" }
	group = {
		name = "layout"
		attach = { chance = 10 here = "sabre_entity" }
		attach = { chance = 10 here = "rifle_entity" }
	}
}

# What cannot be attached.
entity = {
	name = "broken_entity"
	pdxmesh = "block_mesh"
	locator = { name = "here" }
	attach = { here = "nowhere_entity" }
	attach = { nowhere = "block_entity" }
	attach = { here = "loop_entity" }
	attach = { here = "lost_entity" }
	attach = { here = "smoke_entity" }
}
entity = { name = "loop_entity" locator = { name = "here" } attach = { here = "broken_entity" } }
pdxmesh = { name = "lost_mesh" file = "lost.mesh" }
entity = { name = "lost_entity" pdxmesh = "lost_mesh" }
entity = { name = "smoke_entity" }

# A frame whose own mesh is missing, and that attaches a block twice whose
# texture is missing.
pdxmesh = {
	name = "patchy_mesh"
	file = "block.mesh"
	meshsettings = { name = "quadShape" index = 0 texture_diffuse = "missing.png" }
}
entity = { name = "patchy_entity" pdxmesh = "patchy_mesh" }
entity = {
	name = "frame_entity"
	pdxmesh = "undefined_mesh"
	locator = { name = "left" position = { -5 0 0 } }
	locator = { name = "right" position = { 5 0 0 } }
	attach = { left = "patchy_entity" right = "patchy_entity" }
}
`

// rigMesh is a quad with a skeleton of one bone, five up, and two locators:
// one twenty along z, one three along x from the bone.
func rigMesh() []byte {
	return meshtest.New().
		Object(1, "object").Object(2, "quadShape").Mesh(meshtest.Quad(2, 2), "rig.dds").
		Object(3, "skeleton").Object(4, "spine").Ints("ix", 0).Floats("tx", 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, -5, 0).
		Object(1, "locator").
		Object(2, "top").Floats("p", 0, 0, 20).Floats("q", 0, 0, 0, -1).
		Object(2, "hand").Floats("p", 3, 0, 0).Floats("q", 0, 0, 0, 1).Strings("pa", "spine").
		Bytes()
}

func townLoader(t *testing.T) *Loader {
	t.Helper()

	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/town/town.asset":        []byte(townAsset),
		"gfx/models/town/block.mesh":        meshtest.QuadFile(2, 2),
		"gfx/models/town/rig.mesh":          rigMesh(),
		"gfx/models/town/block_diffuse.png": picture(t, red),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})

	return NewLoader(set, asset.Load(set))
}

// centres are where the parts of each entity of a model are, by the middle of
// the box each fits in, in the model's coordinates, which mirror x.
func centres(built *model.Model) map[string][][3]float32 {
	found := map[string][][3]float32{}

	for _, part := range built.Parts {
		box := &model.Model{Parts: []model.Part{part}}
		box.Bounds()

		var centre [3]float32
		for axis := range 3 {
			centre[axis] = float32(math.Round(float64(box.Min[axis]+box.Max[axis])/2*1000) / 1000)
		}

		found[part.Entity] = append(found[part.Entity], centre)
	}

	return found
}

func extent(part model.Part) [3]float32 {
	box := &model.Model{Parts: []model.Part{part}}
	box.Bounds()

	return [3]float32{box.Max[0] - box.Min[0], box.Max[1] - box.Min[1], box.Max[2] - box.Min[2]}
}

// An entity of nothing but attachments draws them, each at its locator:
// moved, scaled by its entity, turned by its locator.
func TestAttachAtLocatorsOfTheEntity(t *testing.T) {
	built, diagnostics, err := townLoader(t).Load("square_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(diagnostics) != 0 {
		t.Errorf("diagnostics = %v", diagnostics)
	}

	want := []model.Attachment{
		{Entity: "big_block_entity", To: "square_entity", Locator: "east"},
		{Entity: "block_entity", To: "square_entity", Locator: "turned"},
	}
	if !slices.Equal(built.Attached, want) {
		t.Errorf("attached = %+v, want %+v", built.Attached, want)
	}

	if len(built.Parts) != 2 {
		t.Fatalf("parts = %d, want one of each block", len(built.Parts))
	}

	big, turned := built.Parts[0], built.Parts[1]

	// Ten along x of the files is ten the other way in the model's.
	if got := centres(built)["big_block_entity"]; len(got) != 1 || got[0] != [3]float32{-10, 0, 0} {
		t.Errorf("big block at %v, want 10 east, mirrored", got)
	}

	if size := extent(big); size != [3]float32{4, 4, 0} {
		t.Errorf("big block of %v, want twice the block", size)
	}

	// The block, two across x, turned a quarter around y lies across z.
	if size := extent(turned); math.Abs(float64(size[0])) > 1e-5 || math.Abs(float64(size[2]-2)) > 1e-5 {
		t.Errorf("turned block of %v, want it across z", size)
	}

	if turned.Shader != "standard" || turned.Textures.Diffuse == nil {
		t.Errorf("an attached part is drawn as its own entity says: %+v", turned)
	}
}

// A locator of the mesh file, one hanging from a bone, and a bone itself,
// whatever its case.
func TestAttachAtLocatorsAndBonesOfTheMesh(t *testing.T) {
	built, diagnostics, err := townLoader(t).Load("rig_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(diagnostics) != 0 {
		t.Errorf("diagnostics = %v", diagnostics)
	}

	got := centres(built)

	if own := got["rig_entity"]; len(own) != 1 || own[0] != [3]float32{} {
		t.Errorf("the rig's own quad at %v", own)
	}

	want := [][3]float32{
		{0, 0, 20}, // at the locator of the file
		{-3, 5, 0}, // at the locator three along x of the bone, five up
		{0, 5, 0},  // at the bone
	}
	if blocks := got["block_entity"]; !slices.Equal(blocks, want) {
		t.Errorf("blocks at %v, want %v", blocks, want)
	}
}

// What is attached to an attachment hangs from it where it is. Each
// attachment says which it hangs from, and each part which attachment it is
// of, by their numbers.
func TestAttachToAttachments(t *testing.T) {
	built, _, err := townLoader(t).Load("district_entity")
	if err != nil {
		t.Fatal(err)
	}

	if got := centres(built)["big_block_entity"]; len(got) != 1 || got[0] != [3]float32{-10, 0, 100} {
		t.Errorf("big block at %v, want 10 east of the square, 100 north", got)
	}

	want := []model.Attachment{
		{Entity: "square_entity", To: "district_entity", Locator: "north", Parent: 0},
		{Entity: "big_block_entity", To: "square_entity", Locator: "east", Parent: 1},
		{Entity: "block_entity", To: "square_entity", Locator: "turned", Parent: 1},
	}
	if !slices.Equal(built.Attached, want) {
		t.Errorf("attached = %+v, want %+v", built.Attached, want)
	}

	var numbers []int
	for _, part := range built.Parts {
		numbers = append(numbers, part.Attachment)
	}

	if !slices.Equal(numbers, []int{2, 3}) {
		t.Errorf("parts of attachments %v, want the big block's, then the block's", numbers)
	}
}

// An entity attached twice has parts of each, apart.
func TestAttachTheSameEntityTwice(t *testing.T) {
	built, _, err := townLoader(t).Load("rig_entity")
	if err != nil {
		t.Fatal(err)
	}

	var numbers []int
	for _, part := range built.Parts {
		numbers = append(numbers, part.Attachment)
	}

	if !slices.Equal(numbers, []int{0, 1, 2, 3}) {
		t.Errorf("parts of attachments %v, want the rig's own and each block's", numbers)
	}
}

// An entity attaches what the entity it clones does, but for an attach block
// of the same name, which it replaces. A group attaches its first choice.
func TestAttachClonesAndGroups(t *testing.T) {
	loader := townLoader(t)

	built, _, err := loader.Load("dragoon_entity")
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, attached := range built.Attached {
		names = append(names, attached.Entity)
	}

	if !slices.Equal(names, []string{"pack_entity", "rifle_entity"}) {
		t.Errorf("attached = %v, want the pack, and the rifle in place of the sabre", names)
	}

	if got := centres(built)["rifle_entity"]; len(got) != 1 || got[0] != [3]float32{-1, 0, 0} {
		t.Errorf("rifle at %v, want it in the hand the dragoon clones", got)
	}

	built, _, err = loader.Load("site_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Attached) != 1 || built.Attached[0].Entity != "sabre_entity" {
		t.Errorf("attached = %+v, want the first choice of the group", built.Attached)
	}
}

// What cannot be attached is left out and reported, and the rest is drawn.
func TestAttachProblems(t *testing.T) {
	built, diagnostics, err := townLoader(t).Load("broken_entity")
	if err != nil {
		t.Fatal(err)
	}

	for _, fragment := range []string{
		"entity broken_entity attaches nowhere_entity, which is not defined; drawn without it",
		"entity broken_entity attaches block_entity, to nowhere, which is neither a locator nor a bone of it; drawn without it",
		"entity loop_entity attaches broken_entity, which it is itself attached to, through broken_entity -> loop_entity; drawn without it",
		"pdxmesh lost_mesh names gfx/models/town/lost.mesh, which none of the folders has: no such mesh file; drawn as nothing",
	} {
		if !mentions(diagnostics, fragment) {
			t.Errorf("no diagnostic mentioning %q; got %v", fragment, diagnostics)
		}
	}

	// Smoke draws no mesh, which is no problem.
	if len(diagnostics) != 4 {
		t.Errorf("diagnostics = %v, want the four problems", diagnostics)
	}

	if len(built.Parts) != 1 || built.Parts[0].Entity != "broken_entity" {
		t.Errorf("parts = %+v, want the entity's own", built.Parts)
	}

	missing := map[string]bool{}
	for _, attached := range built.Attached {
		missing[attached.Entity+" to "+attached.To] = attached.Missing
	}

	want := map[string]bool{
		"nowhere_entity to broken_entity": true,
		"block_entity to broken_entity":   true,
		"loop_entity to broken_entity":    false,
		"broken_entity to loop_entity":    true,
		"lost_entity to broken_entity":    false,
		"smoke_entity to broken_entity":   false,
	}
	if len(missing) != len(want) {
		t.Errorf("attached = %+v", built.Attached)
	}

	for name, gone := range want {
		if missing[name] != gone {
			t.Errorf("%s missing = %v, want %v", name, missing[name], gone)
		}
	}
}

// An entity that attaches others is drawn with them though its own mesh is
// missing, and a problem of an entity attached twice is reported once.
func TestAttachWithoutAMesh(t *testing.T) {
	built, diagnostics, err := townLoader(t).Load("frame_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 2 {
		t.Errorf("parts = %d, want both blocks", len(built.Parts))
	}

	for _, fragment := range []string{
		"entity frame_entity draws no mesh that is defined; drawn without its mesh",
		"entity patchy_entity: texture missing.png is not at gfx/models/town/missing.png in any of the folders; drawn without it",
	} {
		if !mentions(diagnostics, fragment) {
			t.Errorf("no diagnostic mentioning %q; got %v", fragment, diagnostics)
		}
	}

	if len(diagnostics) != 2 {
		t.Errorf("diagnostics = %v, want each problem once", diagnostics)
	}
}
