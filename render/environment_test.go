package render

import (
	"math"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// shed adds up the irradiance's harmonics for a normal, as the shader does.
func shed(irradiance [9][3]float32, normal [3]float64) [3]float64 {
	var light [3]float64

	for index, weight := range harmonics(normal) {
		for channel := range 3 {
			light[channel] += float64(irradiance[index][channel]) * weight
		}
	}

	return light
}

// An environment of white all round sheds the light of a whole hemisphere,
// pi, on a surface facing any way.
func TestIrradianceOfUniformLight(t *testing.T) {
	pixels := texture.NewPixels(64, 32)
	for index := range pixels.Data {
		pixels.Data[index] = 255
	}

	irradiance := Irradiance(pixels)

	for _, normal := range [][3]float64{{0, 1, 0}, {1, 0, 0}, {0, 0, -1}, {0.6, -0.8, 0}} {
		if light := shed(irradiance, normal); math.Abs(light[0]-math.Pi) > 0.05 {
			t.Errorf("a surface facing %v gets %v, want pi", normal, light[0])
		}
	}
}

// An environment bright above and dark below lights a surface facing up and
// leaves one facing down dark.
func TestIrradianceFollowsTheLight(t *testing.T) {
	pixels := texture.NewPixels(64, 32)

	for y := range 16 {
		for x := range 64 {
			at := pixels.At(x, y)
			pixels.Data[at], pixels.Data[at+1], pixels.Data[at+2] = 255, 255, 255
		}
	}

	irradiance := Irradiance(pixels)
	up, down := shed(irradiance, [3]float64{0, 1, 0}), shed(irradiance, [3]float64{0, -1, 0})

	if up[0] < 2.5 || down[0] > 0.3 {
		t.Errorf("up gets %v, down %v; want nearly pi up and nearly nothing down", up[0], down[0])
	}
}
