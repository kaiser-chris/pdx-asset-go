package texture

import "testing"

// A DXT1 block of red and blue, whose pixels all take the third colour, two
// thirds of the way from the first of them to the second: red comes first, so
// the block is in its four colour form.
var dxt1Block = []byte{
	0x00, 0xf8, // red
	0x1f, 0x00, // blue
	0xaa, 0xaa, 0xaa, 0xaa, // every pixel the third colour, which is 0b10 four times
}

// A DXT5 block of the same two colours, with the alpha of its pixels counting
// up from nothing.
var dxt5Block = []byte{
	255, 0, // alpha from nothing to all of it
	0x88, 0x88, 0x88, 0x88, 0x88, 0x88,
	0x00, 0xf8, 0x1f, 0x00,
	0xaa, 0xaa, 0xaa, 0xaa,
}

// A pixel of a block compressed image is worked out from the block it is in,
// the way the graphics card would work it out as it draws it.
func TestColourOfBlocks(t *testing.T) {
	red := [4]byte{255, 0, 0, 255}
	blue := [4]byte{0, 0, 255, 255}
	mixed := [4]byte{170, 0, 85, 255}

	image := &Image{Width: 4, Height: 4, Format: DXT1, Levels: 1, Data: dxt1Block}

	for _, at := range []struct {
		x, y int
		want [4]byte
	}{
		{0, 0, mixed}, {3, 3, mixed},
	} {
		if got, ok := image.Colour(at.x, at.y); !ok || got != at.want {
			t.Errorf("the pixel at %d,%d is %v, %v; want %v", at.x, at.y, got, ok, at.want)
		}
	}

	// The colours of the first two pixels are the two the block holds, the
	// indices 0b00 and 0b01 of its first byte, and a pixel outside the image
	// is not there at all.
	corner := []byte{0x00, 0xf8, 0x1f, 0x00, 0x04, 0x00, 0x00, 0x00}
	image = &Image{Width: 4, Height: 4, Format: DXT1, Levels: 1, Data: corner}

	if got, _ := image.Colour(0, 0); got != red {
		t.Errorf("the first pixel is %v, want %v", got, red)
	}

	if got, _ := image.Colour(1, 0); got != blue {
		t.Errorf("the second pixel is %v, want %v", got, blue)
	}

	if _, ok := image.Colour(4, 0); ok {
		t.Error("a pixel outside the image was read")
	}

	// A block that holds its alpha: the eight values run from one end of the
	// two it holds to the other.
	image = &Image{Width: 4, Height: 4, Format: DXT5, Levels: 1, Data: dxt5Block}

	if got, _ := image.Colour(0, 0); got[3] != 255 {
		t.Errorf("the first pixel's alpha is %d, want all of it", got[3])
	}

	if got, _ := image.Colour(1, 0); got[3] != 0 {
		t.Errorf("the second pixel's alpha is %d, want none of it", got[3])
	}

	if got, _ := image.Colour(2, 0); got[3] == 0 || got[3] == 255 {
		t.Errorf("the third pixel's alpha is %d, want something between the two", got[3])
	}

	// A DXT3 block holds four bits of alpha per pixel, which is a sixteenth
	// of the way from nothing to all of it.
	dxt3 := []byte{
		0x0f, 0xf0, // the first pixel opaque, the second clear
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0xf8, 0x1f, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	image = &Image{Width: 4, Height: 4, Format: DXT3, Levels: 1, Data: dxt3}

	if got, _ := image.Colour(0, 0); got[3] != 255 {
		t.Errorf("the opaque pixel's alpha is %d, want all of it", got[3])
	}

	if got, _ := image.Colour(1, 0); got[3] != 0 {
		t.Errorf("the clear pixel's alpha is %d, want none of it", got[3])
	}
}

// An image the package has no reader for is not read, which is what a palette
// of a format only the graphics card knows would be.
func TestColourRefusesWhatItCannotRead(t *testing.T) {
	for _, format := range []Format{RGBA16F, Format(0)} {
		image := &Image{Width: 4, Height: 4, Format: format, Levels: 1, Data: make([]byte, 64)}

		if _, ok := image.Colour(0, 0); ok {
			t.Errorf("a %v image was read", format)
		}
	}

	var nothing *Image

	if _, ok := nothing.Colour(0, 0); ok {
		t.Error("a missing image was read")
	}
}
