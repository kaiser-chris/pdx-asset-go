//go:build uitest

package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/texture"
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

// environmentShader returns the sun's colour on the left of the quad, and on
// the right what the environment map holds towards -x once turned by
// CubemapYRotation, the way the games' lighting looks it up.
const environmentShader = `
VertexStruct VS_INPUT
{
	float3 Position : POSITION;
	float2 UV0 : TEXCOORD2;
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

ConstantBuffer( JominiEnvironment )
{
	float3 SunDiffuse;
	float SunIntensity;
	float4x4 CubemapYRotation;
}

PixelShader =
{
	TextureSampler EnvironmentMap
	{
		Ref = JominiEnvironmentMap
		Type = "Cube"
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
				VS_OUTPUT Out;
				Out.Position = mul( ViewProjectionMatrix, float4( Input.Position, 1.0f ) );
				Out.UV0 = Input.UV0;
				return Out;
			}
		]]
	}
}

PixelShader =
{
	MainCode PS_environment
	{
		Input = "VS_OUTPUT"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				if ( Input.UV0.x < 0.5f )
				{
					return float4( SunDiffuse * SunIntensity, 1.0f );
				}

				float3 Direction = mul( (float3x3)CubemapYRotation, float3( -1.0f, 0.0f, 0.0f ) );
				return float4( PdxTexCube( EnvironmentMap, Direction ).rgb, 1.0f );
			}
		]]
	}
}

Effect environment
{
	VertexShader = "VS_quad"
	PixelShader = "PS_environment"
}
`

// The constants of an environment reach the effects, and its environment map
// the sampler of the engine's environment map, looked up the way the game
// looks it up: -x of the mirrored model is +x of the game.
func TestDrawsWithTheEnvironment(t *testing.T) {
	renderer := withRenderer(t)

	files := effectFiles{}
	for name, text := range shaderFiles {
		files[name] = text
	}

	files["gfx/FX/cw/defines_hlsl.fxh"] += `
struct PdxTextureSamplerCube
{
	TextureCube _Texture;
	SamplerState _Sampler;
};
#define PdxTexCube(samp,uv) (samp)._Texture.Sample( (samp)._Sampler, (uv) )
`
	files["gfx/FX/environment.shader"] = environmentShader

	renderer.UseShaders(files)

	// A cube of one pixel a face: +x red, every other face blue.
	cube := &texture.Cube{Size: 1, Format: texture.RGBA8, Levels: 1}
	for face := range 6 {
		if face == 0 {
			cube.Data = append(cube.Data, 255, 0, 0, 255)
		} else {
			cube.Data = append(cube.Data, 0, 0, 255, 255)
		}
	}

	lighting := &environment.Environment{Constants: map[string][]float32{
		"SunDiffuse":   {0, 0.5, 0},
		"SunIntensity": {2},
	}}

	if err := renderer.SetEnvironment(lighting, cube); err != nil {
		t.Fatal(err)
	}

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Shader = "environment"
	source.Parts[0].ShaderFile = "gfx/FX/environment.shader"

	picture := render(t, renderer, source, 0)

	// The left of the texture is the left of the picture, as the front of
	// the quad shows it.
	if left := picture.RGBAAt(20, 32); left.G < 200 || left.R > 50 || left.B > 50 {
		t.Errorf("left of the picture = %v, want the sun's green at its full intensity", left)
	}

	if right := dominant(picture.RGBAAt(44, 32)); right != "red" {
		t.Errorf("right of the picture is %s, want the red +x face of the environment map", right)
	}
}
