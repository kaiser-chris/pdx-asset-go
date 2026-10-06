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
	// ColourMask is the pattern itself: a texture whose colour is what the
	// channel draws. It is nil for a channel the pattern says nothing about,
	// which draws the colour of the palette alone.
	ColourMask *texture.Image

	// Normal and Properties are the surface the pattern brings with it, in
	// the same channels a mesh's own maps are read in, and nil for a channel
	// the pattern says nothing about. Where a channel covers a pixel they are
	// what is drawn there, the pattern being the surface of the accessory
	// rather than a colour laid over it: the maps of the mesh belong to the
	// part the pattern has replaced.
	Normal, Properties *texture.Image

	// Placement is how the pattern is laid over the surface: the zoom, the
	// turn and the offset of the layout the variation names.
	Placement pattern.Placement
}

// AccessoryPalette is one way of colouring an accessory: a colour for each
// channel of the mask, which the games read from a palette texture as many
// pixels wide as the mask has channels, four shades deep, and as many rows
// high as it has shades of its own.
//
// The colours are read out of that texture rather than sampled from it, since
// a part has more textures to draw with than a material has places to put
// them. Which shade and which row are read is the first of each: the games
// pick one at random, and a viewer draws an accessory the same way twice.
type AccessoryPalette struct {
	// Description is the name of the palette's file, which is what tells one
	// alternative from another.
	Description string

	// Colours are the red, green and blue of each channel's colour, from 0 to
	// 1. A palette whose pixels the graphics card would have to decompress
	// could not be read: its colours are then all white, which draws the
	// pattern in its own colour.
	Colours [pattern.Channels][3]float32

	// Read is false for a palette that could not be read.
	Read bool
}

// PaletteColumns is how many shades of each channel's colour a palette holds
// side by side, which is the width of one channel's colours in it.
const PaletteColumns = 4

// PaletteColours reads the colours out of a palette texture: for every channel
// of the mask, the first of the shades the palette holds for it, on the first
// of its rows.
//
// It reports false for a palette whose pixels are not there to be read, which
// is one the graphics card would have to decompress; every colour is then
// white, which draws a pattern in its own colour rather than in none.
func PaletteColours(image *texture.Image) ([pattern.Channels][3]float32, bool) {
	var colours [pattern.Channels][3]float32

	for channel := range pattern.Channels {
		colours[channel] = [3]float32{1, 1, 1}
	}

	if image == nil {
		return colours, false
	}

	read := true

	for channel := range pattern.Channels {
		// A palette 16 pixels wide holds a channel's four shades side by
		// side; the older ones, four wide, hold one shade of each channel.
		column := channel
		if image.Width >= pattern.Channels*PaletteColumns {
			column = channel * PaletteColumns
		}

		pixel, ok := image.Colour(column, 0)
		if !ok {
			read = false

			continue
		}

		for axis := range 3 {
			colours[channel][axis] = float32(pixel[axis]) / 255
		}
	}

	return colours, read
}
