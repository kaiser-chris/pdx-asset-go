package render

import (
	"math"
	"testing"
)

// The points the occlusion samples lie within the disc, each with the height
// of the sphere over it, and spread over all of it.
func TestDiscSamples(t *testing.T) {
	samples := discSamples(16)
	if len(samples) != maxOcclusionSamples*3 {
		t.Fatalf("%d numbers, want %d", len(samples), maxOcclusionSamples*3)
	}

	furthest := 0.0

	for index := range 16 {
		x, y, z := float64(samples[index*3]), float64(samples[index*3+1]), float64(samples[index*3+2])
		radius := math.Hypot(x, y)

		if radius > 1+1e-6 || math.Abs(x*x+y*y+z*z-1) > 1e-5 {
			t.Errorf("sample %d at %v %v %v is not on the unit sphere over the disc", index, x, y, z)
		}

		furthest = math.Max(furthest, radius)
	}

	if furthest < 0.9 {
		t.Errorf("the furthest sample is %v from the middle, want the whole disc covered", furthest)
	}
}

// The blur weighs the pixel in the middle most and the further ones less, and
// nothing past as many as it takes.
func TestBlurWeights(t *testing.T) {
	weights := blurWeights(4, 2)

	if weights[0] != 1 {
		t.Errorf("the middle weighs %v, want 1", weights[0])
	}

	for index := 1; index < 4; index++ {
		if weights[index] <= 0 || weights[index] >= weights[index-1] {
			t.Errorf("weights %v do not fall away from the middle", weights)
		}
	}

	if weights[4] != 0 {
		t.Errorf("weights %v, want none past the fourth", weights)
	}
}
