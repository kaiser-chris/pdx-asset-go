//go:build uitest

package render

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// valley is two squares meeting in a crease down the middle, which lies
// deeper than their outer edges, as seen by the games' camera, which looks
// along z from below zero. Every normal faces the camera, so that the light
// alone does not tell the crease from the rest.
func valley(width, height, depth float32) meshtest.Geometry {
	half := [2]float32{width / 2, height / 2}

	geometry := meshtest.Geometry{
		Positions: []float32{
			-half[0], -half[1], 0, 0, -half[1], depth, -half[0], half[1], 0, 0, half[1], depth,
			0, -half[1], depth, half[0], -half[1], 0, 0, half[1], depth, half[0], half[1], 0,
		},
		UV0:     []float32{0, 1, 0.5, 1, 0, 0, 0.5, 0, 0.5, 1, 1, 1, 0.5, 0, 1, 0},
		Indices: []int32{0, 2, 1, 2, 3, 1, 4, 6, 5, 6, 7, 5},
	}

	for range 8 {
		geometry.Normals = append(geometry.Normals, 0, 0, -1)
		geometry.Tangents = append(geometry.Tangents, 1, 0, 0, 1)
	}

	return geometry
}

// pixelOf is a texture of one pixel.
func pixelOf(colour [4]byte) *texture.Image {
	return &texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: colour[:]}
}

// matteModel is a model of one part of a geometry, white, of no specular light
// and no subsurface scattering, drawn with an effect and darkened towards a
// colour where it is hidden, nil for the black every part has otherwise.
func matteModel(t *testing.T, geometry meshtest.Geometry, shader string, occlusion *texture.Image) *model.Model {
	t.Helper()

	file, err := mesh.Read(meshtest.New().Object(1, "object").Object(2, "s").Mesh(geometry, "").Bytes())
	if err != nil {
		t.Fatal(err)
	}

	built := &model.Model{Name: "matte", Parts: []model.Part{{
		Name:   "matte",
		Shader: shader,
		Pieces: model.Convert(&file.Shapes[0].Meshes[0]),
		Textures: model.Textures{
			Diffuse:    pixelOf([4]byte{255, 255, 255, 0}),
			Properties: pixelOf([4]byte{0, 0, 0, 255}),
			Occlusion:  occlusion,
		},
	}}}
	built.Bounds()

	return built
}

// evenLight lights a model by a white environment alone, as brightly from
// every side.
func evenLight(t *testing.T, renderer *Renderer) *Lighting {
	t.Helper()

	environment, err := renderer.UploadEnvironment(cubeOf([4]byte{255, 255, 255, 255}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(environment.Unload)

	return &Lighting{Environment: environment, EnvironmentIntensity: 0.5, Exposure: 1}
}

// creaseOcclusion is an ambient occlusion that sees a crease of the valley
// the tests draw, which is framed some five units away.
var creaseOcclusion = Occlusion{
	Samples: 16, Radius: 0.6, MaxRadius: 10, DepthDistance: 0.05, MaxDepth: 2,
	Scale: 1, Bias: 0.5, Power: 5, BlendFactor: 0.9,
	Blur: true, BlurWidth: 2, BlurScale: 1, BlurDepthPower: 5, BlurWeights: 4,
}

// The ambient occlusion darkens a crease, which the surfaces either side of
// it hide from the light around, and leaves the open surface as it was.
func TestOcclusionDarkensCreases(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(matteModel(t, valley(2, 2, 0.5), "", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	look := DefaultLook
	look.PaletteColor = [3]float32{1, 1, 1}
	look.Lighting = evenLight(t, renderer)

	open := drawWith(t, renderer, uploaded, look)

	occlusion := creaseOcclusion
	look.Lighting.Occlusion = &occlusion

	occluded := drawWith(t, renderer, uploaded, look)

	crease, side := creaseAndSide(occluded)
	openCrease, openSide := creaseAndSide(open)

	if int(crease.R) > int(openCrease.R)-20 {
		t.Errorf("crease %v with occlusion, %v without; want it darker", crease, openCrease)
	}

	if diff := int(side.R) - int(openSide.R); diff < -4 || diff > 4 {
		t.Errorf("open surface %v with occlusion, %v without; want it as it was", side, openSide)
	}

	// Without any blur, the crease is darkened all the same.
	occlusion.Blur = false

	if unblurred, _ := creaseAndSide(drawWith(t, renderer, uploaded, look)); int(unblurred.R) > int(openCrease.R)-20 {
		t.Errorf("crease %v unblurred, want darker than %v", unblurred, openCrease)
	}
}

// A portrait part is darkened towards the colour of its occlusion texture,
// multiplied by its palette colour, and any other part towards black.
func TestOcclusionDarkensTowardsAColour(t *testing.T) {
	renderer := withRenderer(t)

	red := pixelOf([4]byte{255, 0, 0, 255})

	look := DefaultLook
	look.Lighting = evenLight(t, renderer)
	occlusion := creaseOcclusion
	look.Lighting.Occlusion = &occlusion

	for _, test := range []struct {
		name      string
		shader    string
		texture   *texture.Image
		palette   [3]float32
		wantRed   bool
		wantGreen bool
	}{
		{"skin of a red occlusion", "portrait_skin", red, [3]float32{1, 1, 1}, true, false},
		{"skin of a red occlusion and a green palette", "portrait_skin", red, [3]float32{0, 1, 0}, false, false},
		{"skin of a white occlusion and a green palette", "portrait_skin", pixelOf([4]byte{255, 255, 255, 255}), [3]float32{0, 1, 0}, false, true},
		{"cloth", "portrait_attachment", nil, [3]float32{1, 1, 1}, false, false},
	} {
		uploaded, err := renderer.Upload(matteModel(t, valley(2, 2, 0.5), test.shader, test.texture))
		if err != nil {
			t.Fatal(err)
		}

		look.PaletteColor = [3]float32{1, 1, 1}
		look.Lighting.Occlusion = nil
		open, _ := creaseAndSide(drawWith(t, renderer, uploaded, look))

		look.PaletteColor = test.palette
		look.Lighting.Occlusion = &occlusion
		crease, _ := creaseAndSide(drawWith(t, renderer, uploaded, look))

		uploaded.Unload()

		// The palette colour colours the skin itself as well, where its
		// diffuse map's alpha says, which is nowhere here.
		red, green := int(crease.R) > int(open.R)-10, int(crease.G) > int(open.G)-10
		if red != test.wantRed || green != test.wantGreen {
			t.Errorf("%s: crease %v, %v unoccluded; want red kept %v, green kept %v", test.name, crease, open, test.wantRed, test.wantGreen)
		}

		if crease.B > open.B-20 {
			t.Errorf("%s: crease %v, %v unoccluded; want its blue darkened", test.name, crease, open)
		}
	}
}

// creaseAndSide are the colours of a picture of the valley at its crease, in
// the middle, and on one of its sides.
func creaseAndSide(picture *image.RGBA) (crease, side color.RGBA) {
	bounds := picture.Bounds()
	middle := bounds.Dy() / 2

	return picture.RGBAAt(bounds.Dx()/2, middle), picture.RGBAAt(bounds.Dx()/2-bounds.Dx()/5, middle)
}

// acesCurve is the ACES curve the shader tonemaps by, in Go.
func acesCurve(light float64) float64 {
	v := light * 1.8
	a := v*(v+0.0245786) - 0.000090537
	b := v*(0.983729*v+0.4329510) + 0.238081

	return a / b
}

// The ACES curve brightens the middle of the light and keeps what is brighter
// than white short of it, where no tonemap cuts it off at white.
func TestLightingTonemapsByACES(t *testing.T) {
	renderer := withRenderer(t)

	uploaded, err := renderer.Upload(matteModel(t, meshtest.Quad(2, 2), "", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer uploaded.Unload()

	look := DefaultLook
	look.PaletteColor = [3]float32{1, 1, 1}
	look.Lighting = evenLight(t, renderer)

	for _, light := range []float32{0.18, 4} {
		// The white environment sheds as much light as its intensity on the
		// white quad.
		look.Lighting.EnvironmentIntensity = light

		look.Lighting.ACES = false
		plain := centre(drawWith(t, renderer, uploaded, look))

		look.Lighting.ACES = true
		curved := centre(drawWith(t, renderer, uploaded, look))

		wantPlain := 255 * math.Pow(math.Min(float64(light), 1), 1/gamma)
		wantCurved := 255 * math.Pow(math.Min(acesCurve(float64(light)), 1), 1/gamma)

		if !near(plain.R, int(math.Round(wantPlain))) || !near(curved.R, int(math.Round(wantCurved))) {
			t.Errorf("light %v: %v plain and %v by ACES, want %.0f and %.0f", light, plain.R, curved.R, wantPlain, wantCurved)
		}
	}
}
