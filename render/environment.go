package render

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// The games light a portrait with an environment: a cubemap of its
// surroundings, which lights it softly from every side and which it reflects,
// and a few lights of their own placement, spot lights mostly. Lighting is
// that, for a Look to draw with in the place of the renderer's own studio of
// lights, and the curve the light is turned into colours by.
//
// The cubemap is not uploaded as one: the graphics library binds flat
// textures only. Its faces are laid out as a single picture of longitude and
// latitude, which the shader looks up by direction the same way, its smaller
// copies standing in for the blur of a rough surface's reflection. The light
// it sheds on a surface facing a direction is worked out once, as the nine
// numbers of its spherical harmonics, which the shader adds up for a normal.
//
// The lights and the environment light a surface as the games' engine does,
// in clausewitz/gfx/FX/cw/lighting.fxh and jomini's portrait.shader: the
// falloff of a light, the diffuse and GGX specular of their simple lighting,
// and the environment's light and reflection. Only what the engine sets
// itself, such as its exposure, is fitted to the games' pictures.

// MaxLights is how many lights a Lighting can have; the environments of the
// games have three or four.
const MaxLights = 8

// LightKind is what kind of light a light is.
type LightKind int

const (
	// LightSpot shines from its place along its direction, in a cone.
	LightSpot LightKind = iota

	// LightPoint shines from its place in every direction.
	LightPoint

	// LightDirectional shines along its direction from far away, as the sun.
	LightDirectional
)

// Light is one light, in the coordinates the models are drawn in.
type Light struct {
	Kind LightKind

	// Position is where a spot or point light stands, and Direction which way
	// a spot or directional light shines.
	Position, Direction [3]float32

	// Color is the colour of the light, as bright as it is: above 1 for a
	// light brighter than white, as the games' environments write them.
	Color [3]float32

	// Radius is how far a spot or point light reaches, and Falloff over how
	// much of that its light fades to nothing at the end. A light of no
	// radius reaches everywhere.
	Radius, Falloff float32

	// InnerCone and OuterCone are the angles of a spot light's cone from its
	// direction, in radians: inside the inner it shines fully, outside the
	// outer not at all.
	InnerCone, OuterCone float32

	// AffectedByShadow darkens the light where the Lighting's shadow falls.
	AffectedByShadow bool
}

// Tonemap is the filmic curve of John Hable that turns light into colours:
// its shoulder, its linear part, its toe, and the light it takes for white.
type Tonemap struct {
	ShoulderStrength, LinearStrength, LinearAngle float32
	ToeStrength, ToeNumerator, ToeDenominator     float32
	LinearWhite                                   float32
}

// Lighting is what a model is lit with in the place of the renderer's own
// lights.
type Lighting struct {
	// Lights are the lights, at most MaxLights of them; any beyond are left
	// out.
	Lights []Light

	// Environment is the cubemap of the surroundings, nil for none, and
	// EnvironmentIntensity how brightly it lights.
	Environment          *Environment
	EnvironmentIntensity float32

	// Shadow is where the shadows of the lights affected by them are cast
	// from, nil for none.
	Shadow *Shadow

	// Occlusion darkens the light where the models' surfaces are hidden
	// from the light around them, nil for nowhere.
	Occlusion *Occlusion

	// Tonemap turns the light into colours, nil for none, which shows the
	// light as it is, cut off at white.
	Tonemap *Tonemap

	// ACES turns the light into colours by the ACES filmic curve, as the
	// games' ToneMapAcesFilmic_Hill does, in the place of Tonemap.
	ACES bool

	// Exposure scales the light before it is tonemapped, as the games' fixed
	// exposure does, and ValueScale, SaturationScale and ColorBalance the
	// colours after, in gamma, as their RestoreScene does; zeros leave the
	// light and the colours as they are.
	Exposure                    float32
	ValueScale, SaturationScale float32
	ColorBalance                [3]float32
}

// Environment is a cubemap of a model's surroundings on the GPU, in the games'
// coordinates, as their files hold it.
type Environment struct {
	texture rl.Texture2D
	levels  float32

	// irradiance are the nine spherical harmonics of the light the cubemap
	// sheds on a surface, red, green and blue, each already weighed by how a
	// surface gathers light of its band.
	irradiance [9][3]float32
}

// The size of the picture an environment's cubemap is laid out in: enough for
// the blurred reflections of skin and cloth, which is all a portrait shows.
const (
	environmentWidth  = 512
	environmentHeight = 256
)

// UploadEnvironment lays the faces of a cubemap out as a picture of longitude
// and latitude, works out the light it sheds, and hands the picture to the
// GPU. It needs the OpenGL context.
func (r *Renderer) UploadEnvironment(faces [6]*texture.Image) (*Environment, error) {
	pixels, err := texture.Equirectangular(faces, environmentWidth, environmentHeight)
	if err != nil {
		return nil, err
	}

	uploaded, err := UploadTexture(pixels.Image())
	if err != nil {
		return nil, fmt.Errorf("the environment: %w", err)
	}

	// The picture goes round once across, so the lookup wraps round with it.
	rl.SetTextureWrap(uploaded, rl.WrapRepeat)

	return &Environment{texture: uploaded, levels: float32(max(uploaded.Mipmaps-1, 0)), irradiance: Irradiance(pixels)}, nil
}

// Unload releases the environment's texture. It needs the OpenGL context.
func (e *Environment) Unload() {
	if e != nil && e.texture.ID != 0 {
		rl.UnloadTexture(e.texture)
		e.texture = rl.Texture2D{}
	}
}

// The constants of the first nine real spherical harmonics, and how much of
// each band a surface gathers: the convolution of Ramamoorthi and Hanrahan's
// "An Efficient Representation for Irradiance Environment Maps".
const (
	harmonic0 = 0.282095
	harmonic1 = 0.488603
	harmonic2 = 1.092548
	harmonic3 = 0.315392
	harmonic4 = 0.546274
)

var bandWeights = [9]float64{math.Pi, 2 * math.Pi / 3, 2 * math.Pi / 3, 2 * math.Pi / 3, math.Pi / 4, math.Pi / 4, math.Pi / 4, math.Pi / 4, math.Pi / 4}

// harmonics are the nine spherical harmonics of a direction, in the order the
// shader adds them up in.
func harmonics(direction [3]float64) [9]float64 {
	x, y, z := direction[0], direction[1], direction[2]

	return [9]float64{
		harmonic0,
		harmonic1 * y,
		harmonic1 * z,
		harmonic1 * x,
		harmonic2 * x * y,
		harmonic2 * y * z,
		harmonic3 * (3*z*z - 1),
		harmonic2 * x * z,
		harmonic4 * (x*x - y*y),
	}
}

// Irradiance works out the light a picture of longitude and latitude sheds on
// a surface, as nine spherical harmonics weighed by how a surface gathers each
// band: added up for a normal, as the shader does, they give the light
// falling on a surface facing that way. The picture's colours are taken as
// sRGB, as the games store their cubemaps, and worked out in linear light.
func Irradiance(pixels *texture.Pixels) [9][3]float32 {
	var sums [9][3]float64

	if pixels == nil || pixels.Width <= 0 || pixels.Height <= 0 {
		return [9][3]float32{}
	}

	for y := range pixels.Height {
		v := (float64(y) + 0.5) / float64(pixels.Height)

		// A texel near the poles covers less of the sphere than one at its
		// middle.
		solidAngle := (2 * math.Pi / float64(pixels.Width)) * (math.Pi / float64(pixels.Height)) * math.Sin(v*math.Pi)

		for x := range pixels.Width {
			direction := texture.EquirectangularDirection((float64(x)+0.5)/float64(pixels.Width), v)
			basis := harmonics(direction)
			at := pixels.At(x, y)

			for channel := range 3 {
				light := math.Pow(float64(pixels.Data[at+channel])/255, gamma) * solidAngle

				for index := range basis {
					sums[index][channel] += light * basis[index]
				}
			}
		}
	}

	var irradiance [9][3]float32

	for index := range sums {
		for channel := range 3 {
			irradiance[index][channel] = float32(sums[index][channel] * bandWeights[index])
		}
	}

	return irradiance
}

// The texture unit the environment is bound to: past the sixteen the
// material's maps and a portrait accessory's patterns take. Every desktop
// graphics card of the last decade has thirty-two at least.
const environmentUnit = 16

// The kinds of light as the shader numbers them.
var lightKinds = map[LightKind]float32{LightSpot: 0, LightPoint: 1, LightDirectional: 2}

// applyLighting sets a lighting on the shader, or tells it to light with the
// renderer's own lights for none.
func (r *Renderer) applyLighting(lighting *Lighting) {
	if lighting == nil {
		rl.SetShaderValue(r.shader, r.lit.enabled, []float32{0}, rl.ShaderUniformFloat)

		return
	}

	rl.SetShaderValue(r.shader, r.lit.enabled, []float32{1}, rl.ShaderUniformFloat)

	count := min(len(lighting.Lights), MaxLights)

	var (
		positionRadius         = make([]float32, 0, MaxLights*4)
		colorFalloff           = make([]float32, 0, MaxLights*4)
		directionKind          = make([]float32, 0, MaxLights*4)
		cones                  = make([]float32, 0, MaxLights*2)
		noReach        float32 = 1e9
	)

	for _, light := range lighting.Lights[:count] {
		radius, falloff := light.Radius, light.Falloff
		if radius <= 0 {
			radius, falloff = noReach, 1
		}

		direction := normalized(light.Direction)

		positionRadius = append(positionRadius, light.Position[0], light.Position[1], light.Position[2], radius)
		colorFalloff = append(colorFalloff, light.Color[0], light.Color[1], light.Color[2], max(falloff, 1e-3))
		directionKind = append(directionKind, direction[0], direction[1], direction[2], lightKinds[light.Kind])
		cones = append(cones, float32(math.Cos(float64(light.InnerCone))), float32(math.Cos(float64(light.OuterCone))), boolFloat(light.AffectedByShadow), 0)
	}

	rl.SetShaderValue(r.shader, r.lit.count, []float32{float32(count)}, rl.ShaderUniformFloat)

	if count > 0 {
		rl.SetShaderValueV(r.shader, r.lit.positionRadius, positionRadius, rl.ShaderUniformVec4, int32(count))
		rl.SetShaderValueV(r.shader, r.lit.colorFalloff, colorFalloff, rl.ShaderUniformVec4, int32(count))
		rl.SetShaderValueV(r.shader, r.lit.directionKind, directionKind, rl.ShaderUniformVec4, int32(count))
		rl.SetShaderValueV(r.shader, r.lit.cones, cones, rl.ShaderUniformVec4, int32(count))
	}

	intensity := float32(0)
	if lighting.Environment != nil && lighting.Environment.texture.ID != 0 {
		intensity = lighting.EnvironmentIntensity

		irradiance := make([]float32, 0, 27)
		for _, band := range lighting.Environment.irradiance {
			irradiance = append(irradiance, band[:]...)
		}

		rl.SetShaderValueV(r.shader, r.lit.irradiance, irradiance, rl.ShaderUniformVec3, 9)
		rl.SetShaderValue(r.shader, r.lit.levels, []float32{lighting.Environment.levels}, rl.ShaderUniformFloat)

		rl.EnableShader(r.shader.ID)
		rl.ActiveTextureSlot(environmentUnit)
		rl.EnableTexture(lighting.Environment.texture.ID)
		rl.SetUniform(r.lit.environment, []int32{environmentUnit}, int32(rl.ShaderUniformInt), 1)
		rl.ActiveTextureSlot(0)
	}

	rl.SetShaderValue(r.shader, r.lit.intensity, []float32{intensity}, rl.ShaderUniformFloat)

	curve := [8]float32{}
	if tonemap := lighting.Tonemap; tonemap != nil {
		curve = [8]float32{tonemap.ShoulderStrength, tonemap.LinearStrength, tonemap.LinearAngle, tonemap.ToeStrength,
			tonemap.ToeNumerator, tonemap.ToeDenominator, tonemap.LinearWhite, 1}
	}

	if lighting.ACES {
		curve[7] = 2
	}

	rl.SetShaderValueV(r.shader, r.lit.tonemap, curve[:], rl.ShaderUniformVec4, 2)

	value, saturation, balance := orOne(lighting.ValueScale), orOne(lighting.SaturationScale), lighting.ColorBalance
	if balance == [3]float32{} {
		balance = [3]float32{1, 1, 1}
	}

	rl.SetShaderValue(r.shader, r.lit.grading, []float32{value, saturation, orOne(lighting.Exposure)}, rl.ShaderUniformVec3)
	rl.SetShaderValue(r.shader, r.lit.balance, balance[:], rl.ShaderUniformVec3)
}

// orOne is a scale, or one for none.
func orOne(scale float32) float32 {
	if scale == 0 {
		return 1
	}

	return scale
}

// normalized scales a direction to unit length, and leaves one of no length.
func normalized(direction [3]float32) [3]float32 {
	length := float32(math.Sqrt(float64(direction[0]*direction[0] + direction[1]*direction[1] + direction[2]*direction[2])))
	if length == 0 {
		return direction
	}

	return [3]float32{direction[0] / length, direction[1] / length, direction[2] / length}
}

// lightingLocations are the places in the shader of a lighting's uniforms.
type lightingLocations struct {
	enabled, count                                     int32
	positionRadius, colorFalloff, directionKind, cones int32
	environment, irradiance, levels, intensity         int32
	tonemap, grading, balance                          int32
	shadowed, lightSpace, shadowSoftness, shadowMap    int32
}

// locateLighting finds the uniforms of a lighting in a shader.
func locateLighting(shader rl.Shader) lightingLocations {
	location := func(name string) int32 { return rl.GetShaderLocation(shader, name) }

	return lightingLocations{
		enabled:        location("environmentLit"),
		count:          location("lightCount"),
		positionRadius: location("lightPositionRadius"),
		colorFalloff:   location("lightColorFalloff"),
		directionKind:  location("lightDirectionKind"),
		cones:          location("lightCones"),
		environment:    location("environment"),
		irradiance:     location("irradiance"),
		levels:         location("environmentLevels"),
		intensity:      location("environmentIntensity"),
		tonemap:        location("tonemapCurve"),
		grading:        location("grading"),
		balance:        location("colorBalance"),
		shadowed:       location("shadowed"),
		lightSpace:     location("lightSpace"),
		shadowSoftness: location("shadowSoftness"),
		shadowMap:      location("shadowMap"),
	}
}
