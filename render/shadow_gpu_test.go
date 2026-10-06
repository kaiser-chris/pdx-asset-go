//go:build uitest

package render

import (
	"image"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// shadowEffects are effects that draw the shadow they are in, the way the
// games' do: the point found in the shadow map by ShadowMapTextureMatrix,
// compared with it at the places around it PdxShadowmap gives, and faded
// by ShadowFadeFactor. The blocker casts its shadow with blockerShadow, a
// pixel stage of no output that draws depth alone, offset as the games'
// shadow rasterizer states offset it; the ground has no effect for its
// shadow and casts none.
const shadowEffects = `
VertexStruct VS_INPUT
{
	float3 Position : POSITION;
	float2 UV0 : TEXCOORD2;
	uint4 InstanceIndices : TEXCOORD5;
};

VertexStruct VS_OUTPUT
{
	float4 Position : PDX_POSITION;
	float3 WorldSpacePos : TEXCOORD0;
	float2 UV0 : TEXCOORD1;
};

ConstantBuffer( PdxCamera )
{
	float4x4 ViewProjectionMatrix;
	float4x4 ShadowMapTextureMatrix;
}

ConstantBuffer( PdxMeshInstanceData )
{
	float4 Data[2];
}

ConstantBuffer( PdxShadowmap )
{
	float ShadowFadeFactor;
	float Bias;
	float KernelScale;
	float ShadowScreenSpaceScale;
	int NumSamples;
	float4 DiscSamples[8];
}

PixelShader =
{
	TextureSampler DiffuseMap
	{
		Ref = PdxTexture0
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
		SampleModeU = "Wrap"
		SampleModeV = "Wrap"
	}

	TextureSampler ShadowMap
	{
		Ref = PdxShadowmap
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
		SampleModeU = "Clamp"
		SampleModeV = "Clamp"
		CompareFunction = less_equal
		SamplerType = "Compare"
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
				float4 WorldSpacePos = mul( World, float4( Input.Position, 1.0f ) );

				VS_OUTPUT Out;
				Out.Position = mul( ViewProjectionMatrix, WorldSpacePos );
				Out.WorldSpacePos = WorldSpacePos.xyz;
				Out.UV0 = Input.UV0;
				return Out;
			}
		]]
	}
}

PixelShader =
{
	MainCode PS_lit
	{
		Input = "VS_OUTPUT"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				float4 ShadowProj = mul( ShadowMapTextureMatrix, float4( Input.WorldSpacePos, 1.0f ) );
				ShadowProj.xyz = ShadowProj.xyz / ShadowProj.w;

				float Term = 0.0f;
				for ( int i = 0; i < NumSamples; i++ )
				{
					float4 Samples = DiscSamples[i] * KernelScale;
					Term += PdxTex2DCmpLod0( ShadowMap, ShadowProj.xy + Samples.xy, ShadowProj.z - Bias );
					Term += PdxTex2DCmpLod0( ShadowMap, ShadowProj.xy + Samples.zw, ShadowProj.z - Bias );
				}
				Term = Term * 0.5f / float( NumSamples );
				Term = lerp( 1.0f, Term, ShadowFadeFactor );

				return float4( Term, Term, Term, 1.0f );
			}
		]]
	}

	MainCode PS_shadow
	{
		Input = "VS_OUTPUT"
		Output = "void"
		Code
		[[
			PDX_MAIN
			{
				clip( PdxTex2D( DiffuseMap, Input.UV0 ).a - 0.1f );
			}
		]]
	}
}

RasterizerState ShadowRasterizerState
{
	DepthBias = 0
	SlopeScaleDepthBias = 2
}

Effect ground
{
	VertexShader = "VS_quad"
	PixelShader = "PS_lit"
}

Effect blocker
{
	VertexShader = "VS_quad"
	PixelShader = "PS_lit"
}

Effect blockerShadow
{
	VertexShader = "VS_quad"
	PixelShader = "PS_shadow"
	RasterizerState = ShadowRasterizerState
}
`

// shadowScene is a ground of 2 by 2 facing the camera, and a blocker of 1
// by 1 half a unit in front of its middle, drawn as nothing but its shadow,
// as a part whose settings say shadow_only.
func shadowScene(t *testing.T) *model.Model {
	t.Helper()

	ground := testQuad(t, "ground").Parts[0]
	ground.ShaderFile = "gfx/FX/shadow.shader"
	ground.ShadowShader = model.ShadowEffect(ground.Shader)

	small := quadModel(t, meshtest.Quad(1, 1)).Parts[0]
	for index := range small.Pieces {
		positions := small.Pieces[index].Positions
		for vertex := 2; vertex < len(positions); vertex += 3 {
			positions[vertex] = -0.5
		}
	}

	small.Shader = "blocker"
	small.ShaderFile = "gfx/FX/shadow.shader"
	small.ShadowShader = model.ShadowEffect(small.Shader)
	small.ShadowOnly = true

	scene := &model.Model{Name: "shadow", Parts: []model.Part{ground, small}}
	scene.Bounds()

	return scene
}

// drawShadowScene draws the scene lit from straight in front, along the
// camera, with shadows or without.
func drawShadowScene(t *testing.T, renderer *Renderer, shadows bool) *image.RGBA {
	t.Helper()

	renderer.UseShaders(testFiles(shadowEffects, map[string]string{"gfx/FX/shadow.shader": shadowEffects}))

	lighting := &environment.Environment{
		Constants: map[string][]float32{"ToSunDir": {0, 0, -1}},
		Shadow:    environment.Shadow{KernelScale: 1},
	}
	if err := renderer.SetEnvironment(lighting, nil); err != nil {
		t.Fatal(err)
	}

	uploaded, err := renderer.Upload(shadowScene(t))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	if len(uploaded.Problems) > 0 {
		t.Fatalf("problems = %v", uploaded.Problems)
	}

	viewer := renderer.NewViewer(64, 64)
	defer viewer.Unload()

	viewer.Frame(uploaded.Min, uploaded.Max)
	viewer.Shadows = shadows

	rl.BeginDrawing()
	viewer.Draw(uploaded)
	rl.EndDrawing()

	return viewer.Image()
}

// A part casts its shadow with the effect named after its own, onto what is
// behind it from the sun, which is dark there and lit around; a part drawn
// as its shadow alone is not seen itself. Without shadows, or before the
// shadow map is drawn, every comparison passes.
func TestShadowsAreCast(t *testing.T) {
	renderer := withRenderer(t)

	shaded := drawShadowScene(t, renderer, true)

	if got := centre(shaded); got.R > 40 {
		t.Errorf("behind the blocker = %v, want in its shadow", got)
	}

	// Three quarters of the way to the ground's edge, past the blocker's.
	if got := shaded.RGBAAt(32+18, 32); got.R < 215 {
		t.Errorf("beside the blocker = %v, want lit", got)
	}

	lit := drawShadowScene(t, renderer, false)
	if got := centre(lit); got.R < 250 {
		t.Errorf("without shadows = %v, want lit", got)
	}

	// The shadow map belongs to its frame: what is drawn after it, as a
	// viewer drawn next, compares with none.
	if renderer.shadows.cast {
		t.Error("the shadow map is still taken as cast after the frame")
	}
}

// The parts of no effect of their own, and those whose effect has no effect
// for its shadow, such as the games' decals, cast none.
func TestNothingCastsWithoutAShadowEffect(t *testing.T) {
	renderer := withRenderer(t)

	renderer.UseShaders(testFiles(shadowEffects, map[string]string{"gfx/FX/shadow.shader": shadowEffects}))

	source := shadowScene(t)
	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	casts := map[string]bool{}
	for index, part := range uploaded.parts {
		casts[source.Parts[index].Shader] = part.shadow != nil
	}

	if casts["ground"] || !casts["blocker"] {
		t.Errorf("casting = %v, want the blocker alone", casts)
	}
}
