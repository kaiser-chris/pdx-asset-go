//go:build uitest

package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// effectFiles is a Source of shader files held in memory.
type effectFiles map[string]string

func (f effectFiles) ReadFile(name string) ([]byte, error) {
	if text, ok := f[name]; ok {
		return []byte(text), nil
	}

	return nil, fmt.Errorf("%s, which none of the folders has", name)
}

// The preludes, cut down to what the effect uses, and an effect written the
// way the games write theirs: the camera from a constant buffer, the world
// matrix from the instance data, and the diffuse map through the sampler the
// engine binds the asset's first texture to.
var shaderFiles = effectFiles{
	"gfx/FX/cw/defines_hlsl.fxh": `
#define PDX_POSITION SV_Position
#define PDX_COLOR SV_Target
struct PdxTextureSampler2D
{
	Texture2D _Texture;
	SamplerState _Sampler;
};
#define PdxTex2D(samp,uv) (samp)._Texture.Sample( (samp)._Sampler, (uv) )
float4x4 Create4x4( in float4 x, in float4 y, in float4 z, in float4 w )
{
	return transpose( float4x4( x, y, z, w ) );
}
`,
	"gfx/FX/cw/defines_common.fxh": "",
	"gfx/FX/quad.shader": `
VertexStruct VS_INPUT
{
	float3 Position : POSITION;
	float2 UV0 : TEXCOORD2;
	uint4 InstanceIndices : TEXCOORD5;
};

VertexStruct VS_OUTPUT
{
	float4 Position : PDX_POSITION;
	float2 UV0 : TEXCOORD0;
};

ConstantBuffer( PdxCamera )
{
	float4x4 ViewProjectionMatrix;
}

ConstantBuffer( PdxMeshInstanceData )
{
	float4 Data[2];
}

PixelShader =
{
	TextureSampler DiffuseMap
	{
		Ref = PdxTexture0
	}
}

VertexShader =
{
	MainCode VS_quad
	{
		Input = "VS_INPUT"
		Output = "VS_OUTPUT"
		Code
		[[
			PDX_MAIN
			{
				uint Index = Input.InstanceIndices.y;
				float4x4 World = Create4x4( Data[Index], Data[Index + 1], Data[Index + 2], Data[Index + 3] );

				VS_OUTPUT Out;
				Out.Position = mul( ViewProjectionMatrix, mul( World, float4( Input.Position, 1.0f ) ) );
				Out.UV0 = Input.UV0;
				return Out;
			}
		]]
	}
}

PixelShader =
{
	MainCode PS_quad
	{
		Input = "VS_OUTPUT"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				return PdxTex2D( DiffuseMap, Input.UV0 ) * Tint;
			}
		]]
	}
}

ConstantBuffer( QuadConstants )
{
	float4 Tint;
}

Effect quad
{
	VertexShader = "VS_quad"
	PixelShader = "PS_quad"
}
`,
}

// The quad drawn with an effect of shader files comes out as the renderer's
// own shader draws it: in front of the camera, the right way round, with its
// texture. Tint, a colour no one sets, is white.
func TestDrawsWithTheGamesEffect(t *testing.T) {
	renderer := withRenderer(t)
	renderer.UseShaders(shaderFiles)

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Shader = "quad"
	source.Parts[0].ShaderFile = "gfx/FX/quad.shader"

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	uploaded.Unload()

	if len(uploaded.Problems) > 0 {
		t.Fatalf("problems = %v, want the effect", uploaded.Problems)
	}

	picture := render(t, renderer, source, 0)

	if left, right := dominant(picture.RGBAAt(20, 32)), dominant(picture.RGBAAt(44, 32)); left != "red" || right != "blue" {
		t.Errorf("left of the picture is %s, right %s; want red then blue, as the renderer's own shader draws it", left, right)
	}

	if corner := dominant(picture.RGBAAt(1, 1)); corner != "nothing" {
		t.Errorf("the corner is %s, want the quad to leave it", corner)
	}
}

// A part whose effect cannot be built is drawn with the renderer's own
// shader, and says why.
func TestFallsBackToTheRenderersShader(t *testing.T) {
	renderer := withRenderer(t)
	renderer.UseShaders(shaderFiles)

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Shader = "missing"
	source.Parts[0].ShaderFile = "gfx/FX/quad.shader"

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	if len(uploaded.Problems) != 1 || !strings.Contains(uploaded.Problems[0], "has no effect missing") {
		t.Errorf("problems = %v, want the missing effect", uploaded.Problems)
	}

	if picture := render(t, renderer, source, 0); dominant(picture.RGBAAt(20, 32)) != "red" {
		t.Error("the part was not drawn with the renderer's own shader")
	}
}
