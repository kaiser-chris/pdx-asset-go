package shader

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// files is a Source of shader files held in memory.
type files map[string]string

func (f files) ReadFile(name string) ([]byte, error) {
	if text, ok := f[name]; ok {
		return []byte(text), nil
	}

	return nil, fmt.Errorf("%s, which none of the folders has", name)
}

// The preludes, cut down to what the fixtures use, in the shape the engine
// writes them.
const (
	fixturePreludeHLSL = `
#define PDX_POSITION SV_Position
#define PDX_COLOR SV_Target
#define PDX_COLOR0 SV_Target0

struct PdxTextureSampler2D
{
	Texture2D _Texture;
	SamplerState _Sampler;
};

#define PdxTex2D(samp,uv) (samp)._Texture.Sample( (samp)._Sampler, (uv) )
`

	fixturePreludeCommon = `#define PI 3.14159265359
`

	// The heightmap's shared code, which a stage of the mesh header
	// included before it relies on, as the engine's files do.
	fixtureHeightmap = `
ConstantBuffer( PdxHeightmapConstants )
{
	float2 HeightmapSize;
}

Code
[[
	float GetHeight( float2 Position )
	{
		return Position.x * HeightmapSize.x;
	}
]]
`

	fixtureMesh = `
VertexStruct VS_INPUT
{
	float3 Position : POSITION;
	float3 Normal : TEXCOORD0;
	float2 UV0 : TEXCOORD2;
@ifdef PDX_MESH_UV1
	float2 UV1 : TEXCOORD3;
@endif
	uint4 InstanceIndices : TEXCOORD5;
};

VertexStruct VS_OUTPUT
{
	float4 Position : PDX_POSITION;
	float2 UV0 : TEXCOORD0;
	uint InstanceIndex : TEXCOORD1;
};

ConstantBuffer( PdxCamera )
{
	float4x4 ViewProjectionMatrix;
	float4 Data[2]; # the engine binds more
}

VertexShader =
{
	Code
	[[
		float3 SnapToTerrain( float3 Position )
		{
			return float3( Position.x, GetHeight( Position.xz ), Position.z );
		}
	]]
}
`

	fixtureShader = `
Includes = {
	"cw/mesh.fxh"
	"cw/heightmap.fxh"
}

struct SColors
{
	float4 _Tint; # a comment of the file
};

ConstantBuffer( 5 )
{
	SColors Colors;
}

PixelShader =
{
	TextureSampler DiffuseMap
	{
		Ref = PdxTexture0
		MagFilter = "Linear"
	}
}

Code
[[
	template < typename T >
	T Twice( T Value )
	{
		return Value * 2;
	}
]]

VertexShader =
{
	MainCode VS_standard
	{
		Input = "VS_INPUT"
		Output = "VS_OUTPUT"
		Code
		[[
			PDX_MAIN
			{
				VS_OUTPUT Out;
				float3 Position = SnapToTerrain( Input.Position );
				Out.Position = mul( ViewProjectionMatrix, float4( Position, 1.0f ) ) + Data[Input.InstanceIndices.y + 4];
				Out.UV0 = Input.UV0;
			#ifdef PDX_MESH_UV1
				Out.UV0 += Input.UV1;
			#endif
				Out.InstanceIndex = Input.InstanceIndices.y;
				return Out;
			}
		]]
	}
}

PixelShader =
{
	MainCode PS_standard
	{
		Input = "VS_OUTPUT"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				float4 Color = PdxTex2D( DiffuseMap, Input.UV0 ) * Colors._Tint;
				Color.rgb = Twice( Color.rgb );
			#ifdef BRIGHT
				Color *= BRIGHT;
			#endif
				return PDX_IsFrontFace ? Color : -Color;
			}
		]]
	}
}

RasterizerState TwoSided
{
	CullMode = "none"
}

Effect standard
{
	VertexShader = "VS_standard"
	PixelShader = "PS_standard"
	RasterizerState = TwoSided
	Defines = { "BRIGHT 2.0f" }
}
`
)

func fixtureLibrary() *Library {
	return NewLibrary(files{
		"gfx/FX/cw/defines_hlsl.fxh":   fixturePreludeHLSL,
		"gfx/FX/cw/defines_common.fxh": fixturePreludeCommon,
		"gfx/FX/cw/heightmap.fxh":      fixtureHeightmap,
		"gfx/FX/cw/mesh.fxh":           fixtureMesh,
		"gfx/FX/standard.shader":       fixtureShader,
	})
}

func TestProgram(t *testing.T) {
	program, err := fixtureLibrary().Program("gfx/FX/standard.shader", "standard", []string{"PDX_MESH_UV1"})
	if err != nil {
		var stageErr *StageError
		if errorsAs(err, &stageErr) {
			t.Logf("HLSL of the failing stage:\n%s", map[string]string{"vertex": program.VertexHLSL, "pixel": program.PixelHLSL}[stageErr.Stage])
		}

		t.Fatal(err)
	}

	for _, fragment := range []string{"#version 330", "gl_Position"} {
		if !strings.Contains(program.Vertex, fragment) {
			t.Errorf("the vertex stage has no %q", fragment)
		}
	}

	// The diffuse map is one sampler of the GLSL, tied to its declaration.
	if len(program.Textures) != 1 || program.Textures[0].Type != "sampler2D" || program.Textures[0].Sampler == nil ||
		program.Textures[0].Sampler.Ref != "PdxTexture0" {
		t.Errorf("textures = %+v, want the diffuse map", program.Textures)
	}

	names := []string{}
	for _, uniform := range program.Uniforms {
		names = append(names, uniform.Name)
	}

	// The camera, its array declared longer than written, the heightmap's
	// constants, and the struct of a numbered buffer.
	for _, name := range []string{
		"cb_PdxCamera.ViewProjectionMatrix",
		"cb_PdxCamera.Data[15]",
		"cb_PdxHeightmapConstants.HeightmapSize",
		"cb_PdxConstantBuffer5.Colors._Tint",
	} {
		if !slices.Contains(names, name) {
			t.Errorf("no uniform %s among %v", name, names)
		}
	}

	// The streams of a mesh at the locations raylib binds them to, and what
	// no stream feeds past them.
	locations := map[string]int{}
	for _, attribute := range program.Attributes {
		locations[attribute.Name] = attribute.Location
	}

	if want := map[string]int{"Position": 0, "Normal": 2, "UV0": 1, "UV1": 5, "InstanceIndices": 8}; !mapsEqual(locations, want) {
		t.Errorf("attributes = %v, want %v", locations, want)
	}

	if state := program.States["RasterizerState"]; state == nil || state.Values["CullMode"] != "none" {
		t.Errorf("rasterizer state = %+v, want the two sided one", state)
	}
}

func TestAssembleOrdersSharedCodeFirst(t *testing.T) {
	program, err := fixtureLibrary().Assemble("gfx/FX/standard.shader", "standard", nil)
	if err != nil {
		t.Fatal(err)
	}

	vertex := program.VertexHLSL

	// GetHeight is shared code of the heightmap, included after the mesh
	// header whose vertex code calls it.
	if strings.Index(vertex, "float GetHeight") > strings.Index(vertex, "float3 SnapToTerrain") {
		t.Error("the shared code of a later include comes after the stage code of an earlier one")
	}

	// The struct of the numbered buffer is written before the buffer, and
	// without the file's comment.
	if strings.Index(vertex, "struct SColors") > strings.Index(vertex, "cbuffer PdxConstantBuffer5") {
		t.Error("the buffer comes before the struct its member is of")
	}

	if strings.Contains(vertex, "a comment of the file") {
		t.Error("a # comment of the file made it into the code")
	}

	// Without PDX_MESH_UV1 the mesh has no second set of coordinates.
	if strings.Contains(vertex, "UV1 :") {
		t.Error("the input has UV1 although PDX_MESH_UV1 is not defined")
	}

	if !strings.Contains(program.PixelHLSL, "#define BRIGHT 2.0f") {
		t.Error("the effect's define with a value is not defined")
	}
}

func TestInstantiateTemplates(t *testing.T) {
	code := instantiateTemplates("template < typename T >\nT Twice( T Value ) { if ( true ) { return Value; } return Value * 2; }\nfloat Other;")

	for _, kind := range []string{"float", "float3", "uint4"} {
		if !strings.Contains(code, kind+" Twice( "+kind+" Value )") {
			t.Errorf("no instance for %s in:\n%s", kind, code)
		}
	}

	if strings.Contains(code, "template") || !strings.HasSuffix(code, "float Other;") {
		t.Errorf("code after instantiating:\n%s", code)
	}
}

func TestSplitDefine(t *testing.T) {
	for define, want := range map[string][2]string{
		"TREE":     {"TREE", ""},
		"BRIGHT=2": {"BRIGHT", "2"},
		"BILLBOARD_SCALE_CORRECTION 0.76702f * 2": {"BILLBOARD_SCALE_CORRECTION", "0.76702f * 2"},
		"  ": {"", ""},
	} {
		if name, value := splitDefine(define); name != want[0] || value != want[1] {
			t.Errorf("splitDefine(%q) = %q, %q, want %q", define, name, value, want)
		}
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}

	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			return false
		}
	}

	return true
}

func errorsAs(err error, target **StageError) bool {
	return errors.As(err, target)
}
