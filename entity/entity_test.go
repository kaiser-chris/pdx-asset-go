package entity

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// picture is a one pixel PNG of one colour, which stands in for a texture.
func picture(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, fill)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}

	return encoded.Bytes()
}

// tree writes files below a folder.
func tree(t *testing.T, root string, files map[string][]byte) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

var (
	red   = color.NRGBA{R: 255, A: 255}
	green = color.NRGBA{G: 255, A: 255}
	blue  = color.NRGBA{B: 255, A: 255}
)

// statueAsset defines a statue mesh of one quad, and entities drawing it.
const statueAsset = `
pdxmesh = {
	name = "statue_mesh"
	file = "statue.mesh"
	meshsettings = {
		name = "quadShape"
		index = 0
		texture_diffuse = "statue_diffuse.png"
		texture_normal = "statue_normal.png"
		texture_specular = "../shared/shared_properties.png"
		shader = "standard"
	}
}

entity = { name = "statue_entity" pdxmesh = "statue_mesh" }
entity = { name = "copy_entity" clone = "statue_entity" }
entity = {
	name = "painted_entity"
	pdxmesh = "statue_mesh"
	meshsettings = { name = "QUADSHAPE" index = 0 texture_diffuse = "painted_diffuse.png" shader = "painted" }
}
entity = { name = "silent_entity" }
`

func statueGame(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/statue/statue.asset":                  []byte(statueAsset),
		"gfx/models/statue/statue.mesh":                   meshtest.QuadFile(2, 4),
		"gfx/models/statue/statue_diffuse.png":            picture(t, red),
		"gfx/models/statue/statue_normal.png":             picture(t, blue),
		"gfx/models/statue/painted_diffuse.png":           picture(t, green),
		"gfx/models/shared/shared_properties.png":         picture(t, blue),
		"gfx/models/statue/unrelated/not_a_texture.dds":   []byte("rubbish"),
		"gfx/models/statue/unrelated/unrelated.mesh.text": []byte("rubbish"),
	})

	return root
}

func TestLoadAnEntity(t *testing.T) {
	root := statueGame(t)

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})
	loader := NewLoader(set, asset.Load(set))

	built, diagnostics, err := loader.Load("statue_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(diagnostics) != 0 {
		t.Errorf("unexpected diagnostics: %v", diagnostics)
	}

	if built.Name != "statue_entity" || len(built.Parts) != 1 {
		t.Fatalf("model = %+v", built)
	}

	part := built.Parts[0]
	if part.Name != "quadShape" || part.Shader != "standard" || len(part.Pieces) != 1 || part.Pieces[0].Vertices() != 4 {
		t.Errorf("part = %+v", part)
	}

	textures := part.Textures
	if textures.Diffuse == nil || [4]byte(textures.Diffuse.Data) != [4]byte{255, 0, 0, 255} {
		t.Errorf("diffuse = %+v, want the red picture", textures.Diffuse)
	}

	if textures.Normal == nil || textures.Properties == nil || [4]byte(textures.Properties.Data) != [4]byte{0, 0, 255, 255} {
		t.Errorf("normal %+v and properties %+v, the latter found through ../", textures.Normal, textures.Properties)
	}

	// Two units across and four up, mirrored across x, which a square
	// centred on the origin does not show.
	if built.Min != [3]float32{-1, -2, 0} || built.Max != [3]float32{1, 2, 0} {
		t.Errorf("bounds = %v to %v", built.Min, built.Max)
	}

	// A second load shares the textures the first one decoded.
	again, _, _ := loader.Load("statue_entity")
	if again.Parts[0].Textures.Diffuse != textures.Diffuse {
		t.Error("the texture was decoded again rather than kept")
	}

	loader.Forget()

	if after, _, _ := loader.Load("statue_entity"); after.Parts[0].Textures.Diffuse == textures.Diffuse {
		t.Error("Forget kept the decoded textures")
	}
}

// A clone draws the mesh of the entity it clones, and an entity's own mesh
// settings override those of its mesh, whatever the case of the shape.
func TestLoadFollowsClonesAndOverrides(t *testing.T) {
	root := statueGame(t)

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})
	loader := NewLoader(set, asset.Load(set))

	copied, _, err := loader.Load("copy_entity")
	if err != nil || copied.Parts[0].Textures.Diffuse == nil || copied.Parts[0].Textures.Diffuse.Data[0] != 255 {
		t.Errorf("clone = %+v, %v, want the statue as it is", copied, err)
	}

	painted, _, err := loader.Load("painted_entity")
	if err != nil {
		t.Fatal(err)
	}

	part := painted.Parts[0]
	if part.Shader != "painted" || part.Textures.Diffuse == nil || [4]byte(part.Textures.Diffuse.Data) != [4]byte{0, 255, 0, 255} {
		t.Errorf("painted part = shader %q, diffuse %+v, want the entity's own", part.Shader, part.Textures.Diffuse)
	}

	// The entity's settings replace the mesh's whole, textures it does not
	// name included.
	if part.Textures.Normal != nil {
		t.Error("the overridden settings kept the mesh's normal map")
	}
}

// A mod replacing a mesh or a texture is drawn with its own.
func TestLoadPrefersTheFilesOfMods(t *testing.T) {
	game := statueGame(t)
	mod := filepath.Join(t.TempDir(), "mod")

	tree(t, mod, map[string][]byte{
		"gfx/models/statue/statue.mesh":         meshtest.QuadFile(6, 6),
		"gfx/models/statue/statue_diffuse.png":  picture(t, green),
		"gfx/models/statue/zz_not_related.mesh": meshtest.QuadFile(1, 1),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: game}, {Name: "mod", Path: mod}})
	built, _, err := NewLoader(set, asset.Load(set)).Load("statue_entity")
	if err != nil {
		t.Fatal(err)
	}

	if built.Max != [3]float32{3, 3, 0} {
		t.Errorf("bounds = %v to %v, want the mod's larger quad", built.Min, built.Max)
	}

	if diffuse := built.Parts[0].Textures.Diffuse; diffuse == nil || diffuse.Data[1] != 255 {
		t.Errorf("diffuse = %+v, want the mod's green", diffuse)
	}
}

// What cannot be drawn at all is an error; what can be drawn without some of
// its textures is drawn and reported.
func TestLoadProblems(t *testing.T) {
	root := statueGame(t)

	broken := `
pdxmesh = { name = "lost_mesh" file = "lost.mesh" }
pdxmesh = { name = "garbled_mesh" file = "garbled.mesh" }
pdxmesh = { name = "empty_mesh" file = "empty.mesh" }
pdxmesh = {
	name = "patchy_mesh"
	file = "patchy.mesh"
	meshsettings = { name = "quadShape" texture_diffuse = "missing.png" texture_normal = "broken.png" }
}
pdxmesh = { name = "bare_mesh" file = "patchy.mesh" }
entity = { name = "lost_entity" pdxmesh = "lost_mesh" }
entity = { name = "garbled_entity" pdxmesh = "garbled_mesh" }
entity = { name = "empty_entity" pdxmesh = "empty_mesh" }
entity = { name = "patchy_entity" pdxmesh = "patchy_mesh" }
entity = { name = "bare_entity" pdxmesh = "bare_mesh" }
`

	tree(t, root, map[string][]byte{
		"gfx/models/broken/broken.asset": []byte(broken),
		"gfx/models/broken/garbled.mesh": []byte("@@b@!"),
		"gfx/models/broken/empty.mesh":   meshtest.New().Object(1, "object").Bytes(),
		"gfx/models/broken/patchy.mesh":  meshtest.QuadFile(1, 1),
		"gfx/models/broken/broken.png":   []byte("not a picture"),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})
	loader := NewLoader(set, asset.Load(set))

	for name, fragment := range map[string]string{
		"nothing_entity": "there is no entity nothing_entity",
		"silent_entity":  "entity silent_entity draws no mesh that exists",
		"lost_entity":    "pdxmesh lost_mesh names gfx/models/broken/lost.mesh, which none of the folders has",
		"garbled_entity": "garbled.mesh",
	} {
		if built, _, err := loader.Load(name); err == nil || !strings.Contains(err.Error(), fragment) {
			t.Errorf("%s: %+v, %v, want an error mentioning %q", name, built, err, fragment)
		}
	}

	// A mesh file of locators only is a model of no parts, not an error.
	if empty, diagnostics, err := loader.Load("empty_entity"); err != nil || len(empty.Parts) != 0 || len(diagnostics) != 0 {
		t.Errorf("empty = %+v, %v, %v, want a model of no parts", empty, diagnostics, err)
	}

	built, diagnostics, err := loader.Load("patchy_entity")
	if err != nil {
		t.Fatal(err)
	}

	if textures := built.Parts[0].Textures; textures.Diffuse != nil || textures.Normal != nil {
		t.Errorf("textures = %+v, want both left out", textures)
	}

	for _, fragment := range []string{
		"entity patchy_entity: texture missing.png is not at gfx/models/broken/missing.png in any of the folders; drawn without it",
		"entity patchy_entity: texture broken.png:",
	} {
		if !mentions(diagnostics, fragment) {
			t.Errorf("no diagnostic mentioning %q; got %v", fragment, diagnostics)
		}
	}

	bare, diagnostics, err := loader.Load("bare_entity")
	if err != nil || bare.Parts[0].Shader != "" || !mentions(diagnostics, "pdxmesh bare_mesh has no meshsettings for shape quadShape; drawn untextured") {
		t.Errorf("bare = %+v, %v, %v", bare, diagnostics, err)
	}
}

func mentions(diagnostics report.Diagnostics, fragment string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.String(), fragment) {
			return true
		}
	}

	return false
}

// Only the most detailed shapes are drawn, every mesh of them, each with the
// settings of its index.
func TestLoadEveryMeshOfTheMostDetailedShapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	quad := meshtest.Quad(1, 1)

	file := meshtest.New().Object(1, "object").
		Object(2, "bodyShape").Mesh(quad, "").Mesh(quad, "").
		Object(2, "bodyShape_lod1").Ints("lod", 1).Mesh(quad, "").
		Bytes()

	tree(t, root, map[string][]byte{
		"gfx/models/body/body.mesh": file,
		"gfx/models/body/body.asset": []byte(`
pdxmesh = {
	name = "body_mesh"
	file = "body.mesh"
	meshsettings = { name = "bodyShape" index = 0 shader = "skin" }
	meshsettings = { name = "bodyShape" index = 1 shader = "cloth" }
	meshsettings = { name = "bodyShape_lod1" index = 0 shader = "far" }
}
entity = { name = "body_entity" pdxmesh = "body_mesh" }
`),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})
	built, _, err := NewLoader(set, asset.Load(set)).Load("body_entity")
	if err != nil {
		t.Fatal(err)
	}

	if len(built.Parts) != 2 || built.Parts[0].Shader != "skin" || built.Parts[1].Shader != "cloth" {
		t.Errorf("parts = %+v", built.Parts)
	}
}

// A part keeps what its settings say of how it is drawn: its effect, the
// pass of decals it goes in, as the plantations of Victoria 3 write
// subpass = "LocalDecals" for their ground, and whether it is drawn as
// nothing but its shadow.
func TestLoadKeepsHowAPartIsDrawn(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	tree(t, root, map[string][]byte{
		"gfx/models/estate/estate.asset": []byte(`
pdxmesh = {
	name = "estate_mesh"
	file = "estate.mesh"
	meshsettings = {
		name = "quadShape"
		index = 0
		texture_diffuse = "ground_diffuse.png"
		shader = "decal_local"
		shader_file = "gfx/FX/pdxmesh_decal.shader"
		subpass = "LocalDecals"
	}
}
entity = { name = "estate_entity" pdxmesh = "estate_mesh" }
pdxmesh = {
	name = "estate_shade_mesh"
	file = "estate.mesh"
	meshsettings = {
		name = "quadShape"
		index = 0
		shader = "standard"
		shadow_shader = ""
		shadow_only = yes
	}
}
entity = { name = "estate_shade_entity" pdxmesh = "estate_shade_mesh" }
`),
		"gfx/models/estate/estate.mesh":        meshtest.QuadFile(2, 2),
		"gfx/models/estate/ground_diffuse.png": picture(t, green),
	})

	set := folders.Open([]folders.Source{{Name: "game", Path: root}})

	built, _, err := NewLoader(set, asset.Load(set)).Load("estate_entity")
	if err != nil {
		t.Fatal(err)
	}

	part := built.Parts[0]
	if part.Subpass != "LocalDecals" || !part.IsDecal() {
		t.Errorf("subpass = %q, want the pass of local decals", part.Subpass)
	}

	if part.Shader != "decal_local" || part.ShadowOnly {
		t.Errorf("shader = %q, shadow only %v; want decal_local, drawn", part.Shader, part.ShadowOnly)
	}

	shade, _, err := NewLoader(set, asset.Load(set)).Load("estate_shade_entity")
	if err != nil {
		t.Fatal(err)
	}

	if !shade.Parts[0].ShadowOnly {
		t.Error("a part drawn as its shadow alone is drawn")
	}
}
