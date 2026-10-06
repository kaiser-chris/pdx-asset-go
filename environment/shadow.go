package environment

import (
	"math"

	"github.com/kaiser-chris/pdx-parser-go/script"
)

// The shadows of the sun.
//
// The games cast their shadows from a direction of their own: the direction
// to the sun, offset by shadow_direction_offset, which Victoria 3 uses to
// cast them longer and to the side, Europa Universalis 5 to cast them
// shorter. shadowmap_kernelscale is how far around a point its shadow is
// sampled, in pixels of the shadow map, and shadowmap_depthbias how much
// closer to the sun a point is taken to be as it is compared, which Crusader
// Kings 3 sets against the bias its shadow effects draw the map with.

// Shadow is what an environment file sets for the shadows of the sun.
type Shadow struct {
	// DirectionOffset is added to the direction to the sun, as the file
	// writes it, to give the direction the shadows are cast from.
	DirectionOffset [3]float32

	// KernelScale is how far around a point its shadow is sampled, in
	// pixels of the shadow map.
	KernelScale float32

	// DepthBias is subtracted from the depth of a point as it is compared
	// with the shadow map's, which runs from 0 to 1.
	DepthBias float32
}

// engineShadow is what the engine casts shadows with by keys a file does not
// set: from the sun itself, sampled a pixel around, without a bias.
var engineShadow = Shadow{KernelScale: 1}

// The keys of the shadows.
const (
	shadowOffsetKey = "shadow_direction_offset"
	shadowKernelKey = "shadowmap_kernelscale"
	shadowBiasKey   = "shadowmap_depthbias"
)

// isShadowKey reports whether a key is one of the shadows'.
func isShadowKey(key string) bool {
	return key == shadowOffsetKey || key == shadowKernelKey || key == shadowBiasKey
}

// readShadow reads the shadows of the fields of an environment file, the
// later of two with the same key winning.
func readShadow(fields []script.Field) Shadow {
	shadow := engineShadow

	for _, field := range fields {
		values, ok := numbers(field.Value)
		if !ok {
			continue
		}

		switch {
		case field.Key == shadowOffsetKey && len(values) == 3:
			shadow.DirectionOffset = [3]float32(values)
		case field.Key == shadowKernelKey && len(values) == 1:
			shadow.KernelScale = values[0]
		case field.Key == shadowBiasKey && len(values) == 1:
			shadow.DepthBias = values[0]
		}
	}

	return shadow
}

// ShadowDirection is the direction the shadows are cast from, towards the
// light, of length one, in the game's coordinates: the direction to the sun,
// made of length one as the shaders take it, which the files do not write
// it as, offset by the file's; or straight down for an environment without
// a sun. The offset is one of a direction of length one: Europa Universalis
// 5 offsets the sun its water reflects by 100 up, for straight above.
func (e *Environment) ShadowDirection() [3]float32 {
	var direction [3]float32
	if sun := e.Constants["ToSunDir"]; len(sun) == 3 {
		direction = unit([3]float32(sun))
	}

	for axis := range direction {
		direction[axis] += e.Shadow.DirectionOffset[axis]
	}

	if direction == [3]float32{} {
		return [3]float32{0, 1, 0}
	}

	return unit(direction)
}

// unit is a direction made of length one, or nothing for none.
func unit(direction [3]float32) [3]float32 {
	length := math.Sqrt(float64(direction[0]*direction[0] + direction[1]*direction[1] + direction[2]*direction[2]))
	if length == 0 {
		return direction
	}

	for axis := range direction {
		direction[axis] /= float32(length)
	}

	return direction
}
