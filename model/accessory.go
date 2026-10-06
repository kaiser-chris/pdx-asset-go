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

	// Placement is how the pattern is laid over the surface: the zoom, the
	// turn and the offset of the layout the variation names.
	Placement pattern.Placement
}

// AccessoryPalette is one way of colouring an accessory.
type AccessoryPalette struct {
	// Description is the name of the palette's file, which is what tells one
	// alternative from another.
	Description string

	// Image is the palette: as many pixels wide as the mask has channels
	// times the shades the games choose between, and as many rows high as it
	// has shades of its own.
	Image *texture.Image
}

// PaletteColumns is how many shades of each channel's colour a palette holds
// side by side, which is the width of one channel's colours in it.
const PaletteColumns = 4

// PaletteUV is where a channel of a palette of the given size reads its
// colour: the column of that channel, its first shade, and the first row.
//
// The games pick a shade and a row at random; a viewer draws the first of
// each, so that an accessory looks the same every time it is opened.
func PaletteUV(channel, width, height int) [2]float32 {
	if width <= 0 || height <= 0 {
		return [2]float32{0.5, 0.5}
	}

	column := channel
	if width >= pattern.Channels*PaletteColumns {
		column = channel * PaletteColumns
	}

	return [2]float32{
		(float32(column) + 0.5) / float32(width),
		0.5 / float32(height),
	}
}
