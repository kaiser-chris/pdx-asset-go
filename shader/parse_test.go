package shader

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `
Includes = {
	"cw/pdxmesh.fxh"
	"jomini/jomini_lighting.fxh"
}

supports_additional_shader_options = {
	PDX_MESH_SKINNED
	PDX_MESH_UV1
}

PixelShader =
{
	TextureSampler DiffuseMap
	{
		Index = 0
		Ref = PdxTexture0
		MagFilter = "Linear"
		file = "gfx/models/environment/trees/tree_tint_01.dds"
	}
	MainCode PS_leaf
	{
		Input = "VS_OUTPUT"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				return float4( 1, 0, 0, 1 ); // { not a block }
			}
		]]
	}
}

# A comment, and the semantics of a vertex
VertexStruct VS_OUTPUT
{
	float4 Position : PDX_POSITION;
@ifdef PDX_MESH_UV1
	float2 UV1 : TEXCOORD3;
@endif
};

ConstantBuffer( PdxCamera )
{
	float4x4 ViewProjectionMatrix;
	float4 Data[2]; # a comment
}

BlendState alpha_blend
{
	BlendEnable = yes
	SourceBlend = "SRC_ALPHA"
}

Effect tree
{
	VertexShader = "VS_standard"
	PixelShader = "PS_leaf"
	BlendState = alpha_blend
	Defines = { "TREE" "WIND" }
}
`

func TestParseNodes(t *testing.T) {
	nodes, err := parseNodes(sample)
	if err != nil {
		t.Fatal(err)
	}

	keys := []string{}
	for _, node := range nodes {
		keys = append(keys, node.Key)
	}

	if want := "Includes supports_additional_shader_options PixelShader VertexStruct ConstantBuffer BlendState Effect"; strings.Join(keys, " ") != want {
		t.Fatalf("statements = %v, want %s", keys, want)
	}

	if includes := nodes[0].Items; len(includes) != 2 || includes[1] != "jomini/jomini_lighting.fxh" {
		t.Errorf("includes = %q", includes)
	}

	if options := nodes[1].Items; len(options) != 2 || options[0] != "PDX_MESH_SKINNED" {
		t.Errorf("options = %q", options)
	}

	pixel := nodes[2].Body
	if len(pixel) != 2 || pixel[0].Key != "TextureSampler" || pixel[0].Name != "DiffuseMap" || pixel[1].Key != "MainCode" || pixel[1].Name != "PS_leaf" {
		t.Fatalf("pixel shader = %+v", pixel)
	}

	if sampler := pixel[0].Body; len(sampler) != 4 || sampler[1].Value != "PdxTexture0" || sampler[3].Value != "gfx/models/environment/trees/tree_tint_01.dds" {
		t.Errorf("sampler = %+v", sampler)
	}

	main := pixel[1].Body
	if len(main) != 3 || main[0].Value != "VS_OUTPUT" || main[2].Key != "Code" || !strings.Contains(main[2].Raw, "{ not a block }") {
		t.Errorf("main code = %+v", main)
	}

	if structure := nodes[3]; structure.Name != "VS_OUTPUT" || !strings.Contains(structure.Raw, "@ifdef PDX_MESH_UV1") {
		t.Errorf("vertex struct = %+v", structure)
	}

	if buffer := nodes[4]; buffer.Args != "PdxCamera" || !strings.Contains(buffer.Raw, "float4 Data[2];") {
		t.Errorf("constant buffer = %+v", buffer)
	}

	effect := nodes[6]
	if effect.Name != "tree" || len(effect.Body) != 4 || effect.Body[2].Value != "alpha_blend" || len(effect.Body[3].Items) != 2 {
		t.Errorf("effect = %+v", effect)
	}

	want := 1
	for range strings.Lines(sample[:strings.Index(sample, "Effect tree")]) {
		want++
	}

	if effect.Line != want {
		t.Errorf("the effect is on line %d, want %d", effect.Line, want)
	}
}

func TestParseNodesRefusesBrokenFiles(t *testing.T) {
	for name, source := range map[string]string{
		"unclosed block":         "Effect x { VertexShader = \"a\"",
		"unclosed code":          "Code [[ void main() {}",
		"unclosed string":        "Includes = { \"a.fxh }",
		"stray brace":            "}",
		"unclosed buffer":        "ConstantBuffer( X ) { float a;",
		"unclosed parenth":       "ConstantBuffer( X { float a; }",
		"unclosed assigned code": "Code = [[ void main() {}",
	} {
		if _, err := parseNodes(source); err == nil {
			t.Errorf("%s: parsed without an error", name)
		}
	}
}

// TestParseInstalled parses every shader file of the installations
// PDX_GAME_DIR names, the engine's folders next to their game folder
// included. The three preludes are plain code rather than shader files, and
// are left out.
func TestParseInstalled(t *testing.T) {
	value := os.Getenv("PDX_GAME_DIR")
	if value == "" {
		t.Skip("set PDX_GAME_DIR to one or more game folders to run this test")
	}

	for _, game := range filepath.SplitList(value) {
		install := filepath.Dir(filepath.Clean(game))
		count := 0

		_ = filepath.WalkDir(install, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}

			extension := strings.ToLower(filepath.Ext(path))
			if extension != ".shader" && extension != ".fxh" || isPrelude(path) {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Error(err)

				return nil
			}

			count++

			if _, err := parseNodes(string(data)); err != nil {
				t.Errorf("%s: %v", path, err)
			}

			return nil
		})

		t.Logf("%s: parsed %d shader files", install, count)
	}
}

func isPrelude(path string) bool {
	name := strings.ToLower(filepath.Base(path))

	return strings.HasPrefix(name, "defines_") && strings.HasSuffix(name, ".fxh")
}
