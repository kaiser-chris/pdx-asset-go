package model

import (
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// paletteImage is an image of the colours given, a row at a time.
func paletteImage(colours ...[]byte) *texture.Image {
	data := []byte{}

	width := 0
	for _, row := range colours {
		data = append(data, row...)
		width = max(width, len(row)/4)
	}

	return &texture.Image{Width: width, Height: len(colours), Format: texture.RGBA8, Levels: 1, Data: data}
}

// paletteRow is a colour written as many times over as a palette holds shades
// of it.
func paletteRow(colour []byte, times int) []byte {
	row := []byte{}

	for range times {
		row = append(row, colour...)
	}

	return row
}

// PaletteColours reads a colour for each channel of a mask out of a palette,
// in the shape the game that ships the palette wrote it in.
func TestPaletteColours(t *testing.T) {
	red := []byte{255, 0, 0, 255}
	green := []byte{0, 255, 0, 255}
	blue := []byte{0, 0, 255, 255}
	white := []byte{255, 255, 255, 255}

	want := [3]float32{0, 0, 1}

	// Sixteen pixels wide: four shades of each channel's colour side by side,
	// of which the first is read.
	wide := paletteImage(append(append(paletteRow(red, 4), paletteRow(blue, 4)...), paletteRow(white, 8)...))

	colours, ok := PaletteColours(wide)
	if !ok || colours[0] != [3]float32{1, 0, 0} || colours[1] != want {
		t.Errorf("a palette sixteen wide holds %v, %v; want red then blue", colours, ok)
	}

	// Four pixels wide: one colour for each channel, in order.
	narrow := paletteImage([]byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255})

	if colours, ok := PaletteColours(narrow); !ok || colours[2] != want {
		t.Errorf("the third channel of a palette four wide holds %v, %v; want %v", colours[2], ok, want)
	}

	// Four pixels high: a row for each channel, which some palettes are
	// written as.
	tall := paletteImage(red, green, blue, white)

	if colours, ok := PaletteColours(tall); !ok || colours[2] != want {
		t.Errorf("the third channel of a palette four high holds %v, %v; want %v", colours[2], ok, want)
	}

	// A palette of one pixel colours every channel with it, which is as much
	// as it says.
	single := paletteImage(red)

	colours, ok = PaletteColours(single)
	if !ok || colours[0] != [3]float32{1, 0, 0} || colours[3] != [3]float32{1, 0, 0} {
		t.Errorf("a palette of one pixel holds %v, %v; want its colour for every channel", colours, ok)
	}

	// An image whose pixels are not there to be read leaves every colour
	// white, which draws a pattern in its own colour.
	unreadable := &texture.Image{Width: 4, Height: 1, Format: texture.RGBA16F, Levels: 1, Data: make([]byte, 32)}

	colours, ok = PaletteColours(unreadable)
	if ok {
		t.Error("a palette of a format the package has no reader for was read")
	}

	for channel, colour := range colours {
		if colour != [3]float32{1, 1, 1} {
			t.Errorf("channel %d of an unread palette holds %v, want white", channel, colour)
		}
	}

	if _, ok := PaletteColours(nil); ok {
		t.Error("a missing palette was read")
	}
}
