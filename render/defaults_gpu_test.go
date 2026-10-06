//go:build uitest

package render

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"maps"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// What the renderer gives the games' effects for what the engine would give
// them, and how it reads what they leave to the engine, each drawn with an
// effect written for it, the way the games' files write theirs, on data made
// up to look like theirs. None of it needs a game.

// testPrelude is what the games' defines_hlsl.fxh declares that the effects
// here use.
const testPrelude = `
#define PDX_POSITION SV_Position
#define PDX_COLOR SV_Target
struct PdxTextureSampler2D { Texture2D _Texture; SamplerState _Sampler; };
struct PdxTextureSamplerCube { TextureCube _Texture; SamplerState _Sampler; };
struct PdxTextureSampler2DCmp { Texture2D _Texture; SamplerComparisonState _Sampler; };
#define PdxTex2D(samp,uv) (samp)._Texture.Sample( (samp)._Sampler, (uv) )
#define PdxTex2DLod0(samp,uv) (samp)._Texture.SampleLevel( (samp)._Sampler, (uv), 0 )
#define PdxTexCube(samp,uv) (samp)._Texture.Sample( (samp)._Sampler, (uv) )
#define PdxTexCubeLod(samp,uv,lod) (samp)._Texture.SampleLevel( (samp)._Sampler, (uv), (lod) )
#define PdxTex2DCmpLod0(samp,uv,value) (samp)._Texture.SampleCmpLevelZero( (samp)._Sampler, (uv), (value) )
float4x4 Create4x4( in float4 x, in float4 y, in float4 z, in float4 w )
{
	return transpose( float4x4( x, y, z, w ) );
}
`

// testEffects is a shader file of effects drawing a quad the way the games'
// meshes are drawn, its world matrix from the instance data, each effect a
// pixel stage of its own. declarations go before them: samplers, constant
// buffers and states.
func testEffects(declarations string, pixelStages map[string]string) string {
	file := `
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
	uint InstanceIndex : TEXCOORD1;
};

ConstantBuffer( PdxCamera )
{
	float4x4 ViewProjectionMatrix;
}

ConstantBuffer( PdxMeshInstanceData )
{
	float4 Data[2];
}
` + declarations + `
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
				Out.InstanceIndex = Index;
				return Out;
			}
		]]
	}
}
`

	for name, code := range pixelStages {
		file += `
PixelShader =
{
	MainCode PS_` + name + `
	{
		Input = "VS_OUTPUT"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				` + code + `
			}
		]]
	}
}

Effect ` + name + `
{
	VertexShader = "VS_quad"
	PixelShader = "PS_` + name + `"
}
`
	}

	return file
}

// testFiles are the preludes, the effects, and further files, such as
// textures, by their path.
func testFiles(effects string, more map[string]string) effectFiles {
	files := effectFiles{
		"gfx/FX/cw/defines_hlsl.fxh":   testPrelude,
		"gfx/FX/cw/defines_common.fxh": "",
		"gfx/FX/test.shader":           effects,
	}

	maps.Copy(files, more)

	return files
}

// testQuad is the quad of the tests, drawn with an effect of the test file.
func testQuad(t *testing.T, effect string) *model.Model {
	t.Helper()

	source := quadModel(t, meshtest.Quad(2, 2))
	source.Parts[0].Shader = effect
	source.Parts[0].ShaderFile = "gfx/FX/test.shader"

	return source
}

// drawTest draws a model with the effects of the files, and fails when an
// effect could not be had.
func drawTest(t *testing.T, renderer *Renderer, files effectFiles, source *model.Model) *image.RGBA {
	t.Helper()

	renderer.UseShaders(files)

	uploaded, err := renderer.Upload(source)
	if err != nil {
		t.Fatal(err)
	}

	problems := uploaded.Problems
	uploaded.Unload()

	if len(problems) > 0 {
		t.Fatalf("problems = %v", problems)
	}

	return render(t, renderer, source, 0)
}

// centre is the pixel in the middle of a picture of the tests.
func centre(picture *image.RGBA) color.RGBA {
	return picture.RGBAAt(32, 32)
}

// near reports whether a channel is within a few steps of a value.
func near(got uint8, want int) bool {
	return int(got) >= want-6 && int(got) <= want+6
}

// The instance data past the world matrix and the opacity is the user data
// of the instance, such as the standard of living of a building, which reads
// as zero for a model without any: read as white, it once turned Victoria 3's
// buildings black. The opacity is one.
func TestUserDataIsZero(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects("", map[string]string{
		"userdata": "return float4( Data[Input.InstanceIndex + 5].x, Data[Input.InstanceIndex + 4].x, Data[Input.InstanceIndex + 6].w, 1.0f );",
	}), nil)

	got := centre(drawTest(t, renderer, files, testQuad(t, "userdata")))
	if got.R != 0 || got.G != 255 || got.B != 0 {
		t.Errorf("user data, opacity, user data = %v, want zero, one, zero", got)
	}
}

// A model shown on its own is drawn as the engine draws one in its
// interface, with GUI_SHADER, which leaves out what only the map has.
func TestEffectsAreForTheInterface(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects("", map[string]string{
		"gui": "#ifdef GUI_SHADER\nreturn float4( 0.0f, 1.0f, 0.0f, 1.0f );\n#else\nreturn float4( 1.0f, 0.0f, 0.0f, 1.0f );\n#endif",
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "gui"))); dominant(got) == "red" || got.G < 200 {
		t.Errorf("colour = %v, want the green of GUI_SHADER", got)
	}
}

// Without a shadow map nothing casts a shadow: a comparison against the
// shadow map passes everywhere, and the samples a shadow is averaged over
// are one, not zero, which the shaders divide by.
func TestNothingCastsAShadow(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects(`
ConstantBuffer( PdxShadowmap )
{
	float ShadowFadeFactor;
	int NumSamples;
}

PixelShader =
{
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
`, map[string]string{
		"shadow": `float Term = 0.0f;
				for ( int i = 0; i < NumSamples; i++ )
				{
					Term += PdxTex2DCmpLod0( ShadowMap, Input.UV0, 0.9f );
				}
				Term /= float( NumSamples );
				return float4( Term, Term, Term, 1.0f );`,
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "shadow"))); got.R < 250 || got.G < 250 {
		t.Errorf("shadow term = %v, want fully lit", got)
	}
}

// An effect without blending covers what is behind it whatever alpha it
// writes, as Crusader Kings 3's buildings write their atlas's alpha of zero.
func TestUnblendedEffectsAreOpaque(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects("", map[string]string{
		"clear": "return float4( 1.0f, 0.0f, 0.0f, 0.0f );",
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "clear"))); got.A != 255 || got.R < 250 {
		t.Errorf("colour = %v, want opaque red", got)
	}
}

// A part in a pass of decals is drawn after the rest, over it, blended by
// its alpha, whatever order the parts come in and whatever its effect's
// states say.
func TestDecalsAreBlendedOverTheRest(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects("", map[string]string{
		"ground": "return float4( 1.0f, 0.0f, 0.0f, 1.0f );",
		"decal":  "return float4( 0.0f, 0.0f, 1.0f, 0.5f );",
	}), nil)

	source := testQuad(t, "ground")
	decal := source.Parts[0]
	decal.Shader = "decal"
	decal.Subpass = "LocalDecals"

	// The decal first, so that drawing it in order would hide it.
	source.Parts = []model.Part{decal, source.Parts[0]}

	got := centre(drawTest(t, renderer, files, source))
	if !near(got.R, 128) || !near(got.B, 128) || got.A != 255 {
		t.Errorf("colour = %v, want the blue decal half over the red ground", got)
	}
}

// An effect with alpha to coverage, which tree.shader asks for in a blend
// state named after the kind that no effect of it names, draws where its
// alpha covers at least half of a pixel.
func TestAlphaToCoverage(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects(`
BlendState BlendState
{
	BlendEnable = no
	alphatocoverage = yes
}
`, map[string]string{
		"leaves": "return float4( 1.0f, 0.0f, 0.0f, Input.UV0.x < 0.5f ? 0.3f : 0.8f );",
	}), nil)

	picture := drawTest(t, renderer, files, testQuad(t, "leaves"))
	if left, right := picture.RGBAAt(20, 32), picture.RGBAAt(44, 32); left.A != 0 || right.A != 255 {
		t.Errorf("left %v, right %v; want the faint left dropped and the right drawn", left, right)
	}
}

// A block compressed environment map, whose levels smaller than a block are
// not uploaded, is sampled at its smaller levels as it is at its largest,
// rather than as an incomplete texture, which reads black.
func TestCompressedEnvironmentMapLevels(t *testing.T) {
	renderer := withRenderer(t)

	// Faces of 8 pixels in blocks of solid red: a level of four blocks,
	// then three of one, the last two smaller than a block.
	red := []byte{0x00, 0xF8, 0x00, 0xF8, 0, 0, 0, 0}
	cube := &texture.Cube{Size: 8, Format: texture.DXT1, Levels: 4}

	for _, blocks := range []int{4, 1, 1, 1} {
		for range 6 * blocks {
			cube.Data = append(cube.Data, red...)
		}
	}

	if err := renderer.SetEnvironment(&environment.Environment{Constants: map[string][]float32{}}, cube); err != nil {
		t.Fatal(err)
	}

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler EnvironmentMap
	{
		Ref = JominiEnvironmentMap
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
		Type = "Cube"
	}
}
`, map[string]string{
		"blurred": "return float4( PdxTexCubeLod( EnvironmentMap, float3( 0.0f, 1.0f, 0.0f ), 1.0f ).rgb, 1.0f );",
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "blurred"))); got.R < 240 || got.G > 20 {
		t.Errorf("colour = %v, want the red of the environment map", got)
	}
}

// An environment map of eight bits a channel holds colours in sRGB, which
// are converted as it is sampled, whatever its sampler says: Crusader Kings
// 3 lights by its own twenty times over, which only the converted light
// makes sense of, and its own samplers of the same file say sRGB = yes. One
// of half floats holds linear light, sampled as it is.
func TestEnvironmentMapIsDecodedByItsFormat(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler EnvironmentMap
	{
		Ref = JominiEnvironmentMap
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
		Type = "Cube"
	}
}
`, map[string]string{
		"sky": "return float4( PdxTexCube( EnvironmentMap, float3( 1.0f, 0.0f, 0.0f ) ).rgb, 1.0f );",
	}), nil)

	bytesCube := &texture.Cube{Size: 1, Format: texture.RGBA8, Levels: 1}
	halfCube := &texture.Cube{Size: 1, Format: texture.RGBA16F, Levels: 1}

	for range 6 {
		bytesCube.Data = append(bytesCube.Data, 128, 128, 128, 255)

		// A half of 0x3800 is a half, a one of 0x3c00 one.
		halfCube.Data = binary.LittleEndian.AppendUint16(halfCube.Data, 0x3800)
		halfCube.Data = binary.LittleEndian.AppendUint16(halfCube.Data, 0x3800)
		halfCube.Data = binary.LittleEndian.AppendUint16(halfCube.Data, 0x3800)
		halfCube.Data = binary.LittleEndian.AppendUint16(halfCube.Data, 0x3c00)
	}

	// 128 of 255 in sRGB is 0.216 of linear light.
	for _, test := range []struct {
		name string
		cube *texture.Cube
		want int
	}{
		{"eight bits", bytesCube, 55},
		{"half floats", halfCube, 128},
	} {
		if err := renderer.SetEnvironment(&environment.Environment{Constants: map[string][]float32{}}, test.cube); err != nil {
			t.Fatal(err)
		}

		if got := centre(drawTest(t, renderer, files, testQuad(t, "sky"))); !near(got.R, test.want) {
			t.Errorf("%s: red = %d, want %d", test.name, got.R, test.want)
		}
	}
}

// cubeDDS is a DDS cube map of faces of one pixel of four bytes, stored blue,
// green, red, alpha, the way Victoria 3 stores cubemap_blackbottom.dds.
func cubeDDS(faces [6][4]byte) []byte {
	header := make([]byte, 128)
	copy(header, "DDS ")

	binary.LittleEndian.PutUint32(header[4:], 124)
	binary.LittleEndian.PutUint32(header[8:], 0x100f)
	binary.LittleEndian.PutUint32(header[12:], 1)
	binary.LittleEndian.PutUint32(header[16:], 1)
	binary.LittleEndian.PutUint32(header[28:], 1)
	binary.LittleEndian.PutUint32(header[76:], 32)
	binary.LittleEndian.PutUint32(header[80:], 0x41)
	binary.LittleEndian.PutUint32(header[88:], 32)
	binary.LittleEndian.PutUint32(header[92:], 0x00ff0000)
	binary.LittleEndian.PutUint32(header[96:], 0x0000ff00)
	binary.LittleEndian.PutUint32(header[100:], 0x000000ff)
	binary.LittleEndian.PutUint32(header[104:], 0xff000000)
	binary.LittleEndian.PutUint32(header[108:], 0x1008)
	binary.LittleEndian.PutUint32(header[112:], 0xfe00)

	for _, face := range faces {
		header = append(header, face[2], face[1], face[0], face[3])
	}

	return header
}

// A cube sampler that names a file of its own reads that file, as the
// samplers of Crusader Kings 3's map objects name theirs, rather than white.
func TestCubeSamplersReadTheirFiles(t *testing.T) {
	renderer := withRenderer(t)

	green := [4]byte{0, 255, 0, 255}
	cube := cubeDDS([6][4]byte{green, green, green, green, green, green})

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler SunnyMap
	{
		Index = 28
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
		Type = "Cube"
		File = "gfx/map/environment/sunny.dds"
	}
}
`, map[string]string{
		"sunny": "return float4( PdxTexCube( SunnyMap, float3( 1.0f, 0.0f, 0.0f ) ).rgb, 1.0f );",
	}), map[string]string{"gfx/map/environment/sunny.dds": string(cube)})

	if got := centre(drawTest(t, renderer, files, testQuad(t, "sunny"))); got.G < 250 || got.R > 5 || got.B > 5 {
		t.Errorf("colour = %v, want the green of the file", got)
	}
}

// The diffuse map holds colours, which are converted from sRGB as they are
// sampled; the normal map holds data, which is not.
func TestDiffuseMapsAreDecoded(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler DiffuseMap
	{
		Ref = PdxTexture0
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
	}
	TextureSampler NormalMap
	{
		Ref = PdxTexture2
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
	}
}
`, map[string]string{
		"materials": "return float4( PdxTex2D( DiffuseMap, Input.UV0 ).r, PdxTex2D( NormalMap, Input.UV0 ).r, 0.0f, 1.0f );",
	}), nil)

	grey := &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: []byte{128, 128, 128, 255}}

	source := testQuad(t, "materials")
	source.Parts[0].Textures = model.Textures{Diffuse: grey, Normal: grey}

	if got := centre(drawTest(t, renderer, files, source)); !near(got.R, 55) || !near(got.G, 128) {
		t.Errorf("diffuse, normal = %d, %d; want 55, decoded, and 128, as stored", got.R, got.G)
	}
}

// pngOf is a PNG of a row of colours.
func pngOf(t *testing.T, colors ...color.NRGBA) string {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, len(colors), 1))
	for x, fill := range colors {
		img.SetNRGBA(x, 0, fill)
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}

	return encoded.String()
}

// What the engine lays over a model, such as the colour of a province or the
// mask of a garment's patterns, reads as nothing; what else the engine binds
// and nothing gives reads as white.
func TestEngineTexturesStandIn(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler ProvinceColorTexture
	{
		Ref = JominiProvinceColor
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
	}
	TextureSampler PatternMask
	{
		Ref = PdxMeshCustomTexture0
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
	}
	TextureSampler FlagTexture
	{
		Ref = PdxMeshCustomTexture0
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
	}
}
`, map[string]string{
		"overlays": "return float4( PdxTex2D( ProvinceColorTexture, Input.UV0 ).a, PdxTex2D( FlagTexture, Input.UV0 ).r, PdxTex2D( PatternMask, Input.UV0 ).r, 1.0f );",
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "overlays"))); got.R != 0 || got.G != 255 || got.B != 0 {
		t.Errorf("province, flag, pattern mask = %v, want nothing, white, nothing", got)
	}
}

// The textures Victoria 3 names for its map in maptextures.settings are read
// from the game, as a model reads them by its texture coordinates. The
// colour maps it is tinted with by its place on the map tint nothing, as the
// model editor shows a model on no map: the decals' blend by the terrain's,
// soft light, and the trees' by theirs, converted to sRGB first, overlay,
// leave a colour as it is.
func TestMapTexturesStandIn(t *testing.T) {
	renderer := withRenderer(t)

	red, blue := color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler SolLowTexture
	{
		Ref = SolLowTexture
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
	}
	TextureSampler ColorMapTree
	{
		Ref = ColorMapTree
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
	}
	TextureSampler ColorTexture
	{
		Ref = PdxTerrainColorMap
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
	}
}
`, map[string]string{
		"sol": "return float4( PdxTex2D( SolLowTexture, Input.UV0 ).rgb, 1.0f );",
		"tree": `float3 Map = pow( PdxTex2D( ColorMapTree, Input.UV0 ).rgb, 1.0f / 2.2f );
				float3 Base = float3( 0.3f, 0.3f, 0.3f );
				return float4( Base * Map * 2.0f, 1.0f );`,
		"decal": `float3 Map = PdxTex2D( ColorTexture, Input.UV0 ).rgb;
				float3 Base = float3( 0.3f, 0.3f, 0.3f );
				return float4( ( 1.0f - 2.0f * Map ) * Base * Base + 2.0f * Base * Map, 1.0f );`,
	}), map[string]string{
		"gfx/map/textures/maptextures.settings": "\"ColormapTree\" = \"colormap_tree.png\"\n\"SolLowTexture\" = \"bronze_diff.png\"\n",
		"gfx/map/textures/colormap_tree.png":    pngOf(t, red, blue),
		"gfx/map/textures/bronze_diff.png":      pngOf(t, red, blue),
	})

	picture := drawTest(t, renderer, files, testQuad(t, "sol"))
	if left, right := dominant(picture.RGBAAt(20, 32)), dominant(picture.RGBAAt(44, 32)); left != "red" || right != "blue" {
		t.Errorf("the texture of the standard of living is %s then %s, want red then blue as in the file", left, right)
	}

	// 0.3 of the base colour, as eight bits.
	for _, effect := range []string{"tree", "decal"} {
		if got := centre(drawTest(t, renderer, files, testQuad(t, effect))); !near(got.R, 77) || got.R != got.B {
			t.Errorf("%s tinted by its colour map = %v, want the base colour of 77 as it is", effect, got)
		}
	}
}

// halfFloatDDS is a DDS picture of one pixel of four half floats of a value,
// the way Crusader Kings 3 stores the table it tonemaps by.
func halfFloatDDS(value uint16) []byte {
	header := make([]byte, 128)
	copy(header, "DDS ")

	binary.LittleEndian.PutUint32(header[4:], 124)
	binary.LittleEndian.PutUint32(header[8:], 0x1007)
	binary.LittleEndian.PutUint32(header[12:], 1)
	binary.LittleEndian.PutUint32(header[16:], 1)
	binary.LittleEndian.PutUint32(header[28:], 1)
	binary.LittleEndian.PutUint32(header[76:], 32)
	binary.LittleEndian.PutUint32(header[80:], 0x4)
	binary.LittleEndian.PutUint32(header[84:], 113)

	for range 4 {
		header = binary.LittleEndian.AppendUint16(header, value)
	}

	return header
}

// A texture of half floats a sampler names for itself, such as Crusader
// Kings 3's table of its tonemap, is uploaded whole, rather than refused for
// pixels twice as wide as most and read as white.
func TestHalfFloatTextures(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler Table
	{
		Index = 10
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
		File = "gfx/FX/table.dds"
	}
}
`, map[string]string{
		"table": "return float4( PdxTex2D( Table, Input.UV0 ).rgb, 1.0f );",
	}), map[string]string{"gfx/FX/table.dds": string(halfFloatDDS(0x3800))})

	// 0x3800 is a half.
	if got := centre(drawTest(t, renderer, files, testQuad(t, "table"))); !near(got.R, 128) || !near(got.G, 128) {
		t.Errorf("colour = %v, want the half of the table", got)
	}
}

// The constants the games' shaders share are set from the graphics defines:
// Victoria 3's MESHTINT_COLOR is the colour the bottom of a building is
// tinted with, its own defaults when the game has none.
func TestSharedConstantsComeFromTheDefines(t *testing.T) {
	renderer := withRenderer(t)

	if err := renderer.SetEnvironment(environment.Default(), nil); err != nil {
		t.Fatal(err)
	}

	files := testFiles(testEffects(`
ConstantBuffer( GameSharedConstants )
{
	float4 _MeshTintColor;
	float _MeshTintHeightMax;
}
`, map[string]string{
		"tint": "return float4( _MeshTintColor.rgb, 1.0f );",
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "tint"))); !near(got.R, 51) || !near(got.G, 36) || !near(got.B, 15) {
		t.Errorf("tint = %v, want 0.20 0.14 0.06 of MESHTINT_COLOR", got)
	}
}

// restoreScene is a post effect written the way Jomini's restorescene.shader
// is: the lit picture as its first texture, the exposure and tonemap picked
// by defines or by TonemapIndex, the size of the picture as constants.
const restoreScene = `
ConstantBuffer( PdxConstantBuffer1 )
{
	float2 ScreenResolution;
	float2 InvScreenResolution;
	float FixedExposureValue;
	float TonemapIndex;
}

VertexStruct VS_OUTPUT_FULLSCREEN
{
	float4 position : PDX_POSITION;
	float2 uv : TEXCOORD0;
};

VertexShader =
{
	VertexStruct VS_INPUT_FULLSCREEN
	{
		int2 position : POSITION;
	};

	MainCode VertexShaderFullscreen
	{
		Input = "VS_INPUT_FULLSCREEN"
		Output = "VS_OUTPUT_FULLSCREEN"
		Code
		[[
			PDX_MAIN
			{
				VS_OUTPUT_FULLSCREEN Out;
				Out.position = float4( Input.position, 0.0, 1.0 );
				Out.uv = Input.position * 0.5 + 0.5;
				return Out;
			}
		]]
	}
}

PixelShader =
{
	TextureSampler MainScene
	{
		Index = 0
		MagFilter = "Point"
		MinFilter = "Point"
		MipFilter = "Point"
		SampleModeU = "Clamp"
		SampleModeV = "Clamp"
	}

	MainCode PixelShader
	{
		Input = "VS_OUTPUT_FULLSCREEN"
		Output = "PDX_COLOR"
		Code
		[[
			PDX_MAIN
			{
				float4 color = PdxTex2DLod0( MainScene, Input.uv );
			#ifdef EXPOSURE_FIXED
				color.r *= FixedExposureValue;
			#endif
			#ifdef TONEMAP_UNCHARTED
				color.g = 1.0;
			#else
				color.g = TonemapIndex * 255.0 / 10.0;
			#endif
				color.b = InvScreenResolution.x * ScreenResolution.x;
				return color;
			}
		]]
	}
}

Effect RestoreAlpha
{
	VertexShader = "VertexShaderFullscreen"
	PixelShader = "PixelShader"
}
`

// The lit picture is turned into the one shown by the game's own post
// effect, with the exposure and tonemap the environment picks: by a define
// where the effect has no TonemapIndex, by TonemapIndex where it has, and
// with the size of the picture.
func TestPostEffect(t *testing.T) {
	renderer := withRenderer(t)

	effects := testEffects("", map[string]string{"lit": "return float4( 0.25f, 0.0f, 0.0f, 1.0f );"})

	for _, test := range []struct {
		name    string
		tonemap string
		post    string
		green   int
	}{
		// The define picks the tonemap: the effect has no TonemapIndex.
		{"by define", "Uncharted", restoreScene, 255},

		// TonemapIndex picks it, as number 7 of the list, TonyMcMapface.
		{"by index", "TonyMcMapface", restoreScene, 178},
	} {
		post := test.post
		if test.name == "by define" {
			post = stripLine(post, "\tfloat TonemapIndex;")
			post = stripLine(post, "\t\t\t\tcolor.g = TonemapIndex * 255.0 / 10.0;")
		}

		lighting := environment.Default()
		lighting.Post.Tonemap = test.tonemap
		lighting.Post.Constants["FixedExposureValue"] = []float32{2}

		if err := renderer.SetEnvironment(lighting, nil); err != nil {
			t.Fatal(err)
		}

		files := testFiles(effects, map[string]string{"gfx/FX/jomini/restorescene.shader": post})
		got := centre(drawTest(t, renderer, files, testQuad(t, "lit")))

		viewer := renderer.NewViewer(8, 8)
		if err := viewer.PostError(); err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		viewer.Unload()

		if !near(got.R, 128) || !near(got.G, test.green) || got.B < 250 {
			t.Errorf("%s: colour = %v, want the red doubled by the exposure, green %d for the tonemap, and the size one over its inverse",
				test.name, got, test.green)
		}
	}
}

// stripLine removes a line from a text.
func stripLine(text, line string) string {
	return string(bytes.Replace([]byte(text), []byte(line+"\n"), nil, 1))
}

// The decals of a building are drawn over those of the ground, whatever
// order the parts come in: Victoria 3's academy lays its garden, a local
// decal, over the cobbles of the world's.
func TestLocalDecalsLieOverTheGround(t *testing.T) {
	renderer := withRenderer(t)

	files := testFiles(testEffects("", map[string]string{
		"cobbles": "return float4( 1.0f, 0.0f, 0.0f, 1.0f );",
		"garden":  "return float4( 0.0f, 1.0f, 0.0f, 1.0f );",
	}), nil)

	source := testQuad(t, "garden")
	garden := source.Parts[0]
	garden.Subpass = "LocalDecals"

	cobbles := garden
	cobbles.Shader = "cobbles"
	cobbles.Subpass = "Decals"

	source.Parts = []model.Part{garden, cobbles}

	if got := centre(drawTest(t, renderer, files, source)); dominant(got) == "red" || got.G < 250 {
		t.Errorf("colour = %v, want the green garden over the red cobbles", got)
	}
}

// An environment without an environment map gives no light from around:
// the sampler of the engine's map reads black, as Victoria 3 draws its
// environment_greyscale.txt, which names none.
func TestEnvironmentWithoutAMapIsDark(t *testing.T) {
	renderer := withRenderer(t)

	if err := renderer.SetEnvironment(&environment.Environment{Constants: map[string][]float32{}}, nil); err != nil {
		t.Fatal(err)
	}

	files := testFiles(testEffects(`
PixelShader =
{
	TextureSampler EnvironmentMap
	{
		Ref = JominiEnvironmentMap
		MagFilter = "Linear"
		MinFilter = "Linear"
		MipFilter = "Linear"
		Type = "Cube"
	}
}
`, map[string]string{
		"sky": "return float4( PdxTexCube( EnvironmentMap, float3( 0.0f, 1.0f, 0.0f ) ).rgb, 1.0f );",
	}), nil)

	if got := centre(drawTest(t, renderer, files, testQuad(t, "sky"))); got.R > 3 || got.G > 3 || got.B > 3 {
		t.Errorf("colour = %v, want black", got)
	}
}
