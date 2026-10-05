package glslcross

import (
	"errors"
	"strings"
	"testing"
)

// The way the games write a stage: structs with semantics, a constant
// buffer, and a texture and its sampler held together in a struct, which
// functions take as one.
const vertexSource = `
struct VS_INPUT
{
	float3 Position : POSITION;
	float2 UV0 : TEXCOORD0;
	uint4 InstanceIndices : TEXCOORD5;
};

struct VS_OUTPUT
{
	float4 Position : SV_Position;
	float2 UV0 : TEXCOORD0;
};

cbuffer PdxCamera
{
	float4x4 ViewProjectionMatrix;
	float4 Data[2];
};

VS_OUTPUT main_entry( VS_INPUT Input )
{
	VS_OUTPUT Out;
	uint Index = Input.InstanceIndices.y + 1;
	Out.Position = mul( ViewProjectionMatrix, float4( Input.Position, 1.0f ) ) + Data[Index];
	Out.UV0 = Input.UV0;
	return Out;
}
`

const pixelSource = `
struct PdxTextureSampler2D
{
	Texture2D _Texture;
	SamplerState _Sampler;
};

PdxTextureSampler2D DiffuseMap;

struct VS_OUTPUT
{
	float4 Position : SV_Position;
	float2 UV0 : TEXCOORD0;
};

float4 Sample( PdxTextureSampler2D Map, float2 UV )
{
	return Map._Texture.Sample( Map._Sampler, UV );
}

float4 main_entry( VS_OUTPUT Input ) : SV_Target
{
	float4 Color = Sample( DiffuseMap, Input.UV0 );
	clip( Color.a - 0.5f );
	return Color;
}
`

func TestCompileVertex(t *testing.T) {
	result, err := Compile(vertexSource, Vertex, "main_entry")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]Uniform{
		"cb_PdxCamera.ViewProjectionMatrix": {Name: "cb_PdxCamera.ViewProjectionMatrix", Kind: Float, VectorSize: 4, Columns: 4},
		"cb_PdxCamera.Data[1]":              {Name: "cb_PdxCamera.Data[1]", Kind: Float, VectorSize: 4, Columns: 1},
	}

	found := 0

	for _, uniform := range result.Uniforms {
		if expected, ok := want[uniform.Name]; ok {
			found++

			if uniform != expected {
				t.Errorf("uniform = %+v, want %+v", uniform, expected)
			}
		}
	}

	if found != len(want) || len(result.Uniforms) != 3 {
		t.Errorf("uniforms = %+v, want the matrix and the two elements of Data", result.Uniforms)
	}

	for _, fragment := range []string{"#version 330", "void main()", "gl_Position", "uniform PdxCamera cb_PdxCamera;", "pdx_varying_0"} {
		if !strings.Contains(result.GLSL, fragment) {
			t.Errorf("the GLSL has no %q:\n%s", fragment, result.GLSL)
		}
	}
}

func TestCompilePixel(t *testing.T) {
	result, err := Compile(pixelSource, Pixel, "main_entry")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Samplers) != 1 {
		t.Fatalf("samplers = %+v, want the diffuse map's", result.Samplers)
	}

	sampler := result.Samplers[0]
	if !strings.Contains(result.GLSL, "uniform sampler2D "+sampler.Name) {
		t.Errorf("the GLSL does not declare %s:\n%s", sampler.Name, result.GLSL)
	}

	if !strings.Contains(sampler.Image, "DiffuseMap") || !strings.Contains(sampler.Sampler, "DiffuseMap") {
		t.Errorf("sampler = %+v, want it named after the diffuse map", sampler)
	}

	if !strings.Contains(result.GLSL, "discard") {
		t.Errorf("clip did not become a discard:\n%s", result.GLSL)
	}
}

func TestCompileReportsErrors(t *testing.T) {
	_, err := Compile("float4 main_entry() : SV_Target { return undefined; }", Pixel, "main_entry")

	var compileErr *Error
	if !errors.As(err, &compileErr) || compileErr.Step != "parse" || !strings.Contains(compileErr.Log, "undefined") {
		t.Errorf("error = %v, want a parse error naming the undefined variable", err)
	}
}
