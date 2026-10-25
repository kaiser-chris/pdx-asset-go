package model

import (
	"github.com/kaiser-chris/pdx-asset-go/pattern"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// Accessory is how a part is coloured and patterned: the portrait accessory
// variation the entity's game data names.
//
// A variation colours an accessory by laying a pattern over it and masking the
// result through the four channels of the entity's own pattern mask: each
// channel draws the pattern of its layer, in the colour its palette holds for
// that channel. The games pick one of a variation's patterns and one of its
// palettes at random for every portrait they draw, so every alternative is
// read here and an application chooses between them.
type Accessory struct {
	// Mask is the entity's pattern mask, whose four channels say where each
	// layer of the pattern is drawn and how strongly.
	Mask *texture.Image

	// Variation is the name of the variation, which is what the alternatives
	// below are of.
	Variation string

	// Patterns and Palettes are the alternatives the games choose between,
	// and Pattern and Palette which of them is drawn: the first of each,
	// where nothing has been chosen.
	Patterns []AccessoryPattern
	Palettes []AccessoryPalette

	Pattern, Palette int
}

// Chosen returns the pattern and the palette that are drawn, and whether the
// accessory has either.
func (a *Accessory) Chosen() (AccessoryPattern, AccessoryPalette, bool) {
	if a == nil {
		return AccessoryPattern{}, AccessoryPalette{}, false
	}

	return a.Alternative(a.Pattern, a.Palette)
}

// Alternative returns one pattern and one palette of the variation, by their
// places among the alternatives it offers, and whether it has both.
func (a *Accessory) Alternative(pattern, palette int) (AccessoryPattern, AccessoryPalette, bool) {
	if a == nil || pattern < 0 || pattern >= len(a.Patterns) || palette < 0 || palette >= len(a.Palettes) {
		return AccessoryPattern{}, AccessoryPalette{}, false
	}

	return a.Patterns[pattern], a.Palettes[palette], true
}

// AccessoryPattern is one way of laying a pattern over an accessory.
type AccessoryPattern struct {
	// Description names the patterns it draws with, which is what tells one
	// alternative from another where they are listed.
	Description string

	// Layers are what each channel of the mask draws with, in the order the
	// games name the channels.
	Layers [pattern.Channels]AccessoryLayer
}

// AccessoryLayer is what one channel of a pattern mask draws with.
type AccessoryLayer struct {
	// ColourMask is the pattern itself: each of its four channels says how
	// strongly one of the palette's four shades of the layer's colour is
	// drawn, the games' portrait_accessory_variation.fxh blending them in
	// one after another. It is nil for a channel the pattern says nothing
	// about, which draws the first shade alone.
	ColourMask *texture.Image

	// Normal and Properties are the surface the pattern brings with it, in
	// the same channels a mesh's own maps are read in, and nil for a channel
	// the pattern says nothing about. Its normal map is laid over the mesh's,
	// whose folds stay, and its properties map takes the place of the
	// mesh's, as the games do.
	Normal, Properties *texture.Image

	// Placement is how the pattern is laid over the surface: the zoom, the
	// turn and the offset of the layout the variation names.
	Placement pattern.Placement
}

// AccessoryPalette is one way of colouring an accessory: four shades of a
// colour for each channel of the mask, which the games read from a palette
// texture sixteen pixels wide, the shades of each channel side by side, and as
// many rows high as it has alternatives of its own.
//
// The colours are read out of that texture rather than sampled from it, since
// a part has more textures to draw with than a material has places to put
// them. Which row is read is the first: the games pick one at random, and a
// viewer draws an accessory the same way twice.
type AccessoryPalette struct {
	// Description is the name of the palette's file, which is what tells one
	// alternative from another.
	Description string

	// Colours are the red, green and blue of each shade of each channel's
	// colour, from 0 to 1. A palette whose pixels the graphics card would
	// have to decompress could not be read: its colours are then all white,
	// which leaves the part in its own colour.
	Colours [pattern.Channels][PaletteColumns][3]float32

	// Read is false for a palette that could not be read.
	Read bool
}

// PaletteColumns is how many shades of each channel's colour a palette holds
// side by side, which is the width of one channel's colours in it.
const PaletteColumns = 4

// PaletteColours reads the colours out of a palette texture: for every channel
// of the mask, the shades the palette holds for it, on the first of the rows
// it holds them in.
//
// A palette is written the way the game that ships it writes it: sixteen
// pixels wide, the four shades of each channel side by side; four wide, one
// pixel for each channel, which is every shade of it; or, for a few of them,
// four pixels high, one row for each channel.
//
// It reports false for a palette whose pixels are not there to be read, which
// is one the graphics card would have to decompress; every colour is then
// white, which leaves a part in its own colour rather than in none.
func PaletteColours(image *texture.Image) ([pattern.Channels][PaletteColumns][3]float32, bool) {
	var colours [pattern.Channels][PaletteColumns][3]float32

	for channel := range pattern.Channels {
		for shade := range PaletteColumns {
			colours[channel][shade] = [3]float32{1, 1, 1}
		}
	}

	if image == nil {
		return colours, false
	}

	read := true

	for channel := range pattern.Channels {
		for shade := range PaletteColumns {
			column, row := palettePlace(image, channel, shade)

			pixel, ok := image.Colour(column, row)
			if !ok {
				read = false

				continue
			}

			for axis := range 3 {
				colours[channel][shade][axis] = float32(pixel[axis]) / 255
			}
		}
	}

	return colours, read
}

// palettePlace is where in a palette one shade of the colour of one channel
// of a mask is.
func palettePlace(image *texture.Image, channel, shade int) (column, row int) {
	switch {
	case image.Width >= pattern.Channels*PaletteColumns:
		return channel*PaletteColumns + shade, 0
	case image.Width >= pattern.Channels:
		return channel, 0
	case image.Height >= pattern.Channels:
		return 0, channel
	}

	// A palette of one pixel colours every channel with it, which is as much
	// as such a palette can say.
	return 0, 0
}
