// Package pattern reads the games' accessory variations: what colours a
// portrait accessory, and in what pattern.
//
// A variation names the ways one accessory can be coloured. Each of its
// patterns lays a pattern texture over the surface through four channels,
// which the entity's own pattern mask selects between, and each of its colour
// palettes holds a colour for every channel of that mask, four shades deep.
// The games pick one pattern and one palette at random for every portrait they
// draw; a viewer offers them instead.
//
// The files are read from gfx/portraits/accessory_variations of a game's
// folders, so a mod that adds variations of its own is read with them.
package pattern

import (
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"
	"github.com/kaiser-chris/pdx-parser-go/script"
)

// variationFolder is where a game keeps the variations, below its root.
const variationFolder = "gfx/portraits/accessory_variations"

// The channels of a pattern mask, in the order the games write them. A channel
// picks the pattern of that name and the colour palette column that goes with
// it.
const (
	Red = iota
	Green
	Blue
	Alpha

	// Channels is how many there are.
	Channels
)

// Range is a value the games pick at random when they draw an accessory.
type Range struct {
	Min, Max float64
}

// Middle is the value between the two. It is what a viewer draws with, where
// the game would roll the dice: the look of a range is its middle.
func (r Range) Middle() float64 {
	return (r.Min + r.Max) / 2
}

// Placement is one placement of a pattern over a surface: how much a pattern
// texture is zoomed by, turned by, in radians, and moved by, in the texture
// coordinates of the surface.
type Placement struct {
	Scale    float64
	Rotation float64
	Offset   [2]float64
}

// Layout is a named placement a pattern may be drawn with, as the range the
// games pick it from.
type Layout struct {
	Name string

	// Scale, Rotation and Offset are the ranges the four numbers are picked
	// from.
	Scale, Rotation  Range
	OffsetX, OffsetY Range
}

// Pinned is the placement a viewer draws a layout with, where the game would
// pick one at random.
func (l Layout) Pinned() Placement {
	return Placement{
		Scale:    l.Scale.Middle(),
		Rotation: l.Rotation.Middle(),
		Offset:   [2]float64{l.OffsetX.Middle(), l.OffsetY.Middle()},
	}
}

// Textures are one pattern's own textures: the pattern itself, and the surface
// detail it brings with it.
//
// The paths are those the game's files write, below the game's root.
type Textures struct {
	Name string

	// ColourMask is the pattern, sampled through the surface's second set of
	// texture coordinates: what of it is coloured, and with which of the
	// palette's colours.
	ColourMask string

	// Normal and Properties are the surface detail the pattern brings, in the
	// same channels as a mesh's own maps.
	Normal, Properties string
}

// Layer is what one channel of a pattern mask draws with: which pattern
// textures, and how they are laid over the surface.
type Layer struct {
	Textures string
	Layout   string
}

// Pattern is one way of patterning an accessory. Its four layers are the four
// channels of the entity's pattern mask.
type Pattern struct {
	// Weight is how likely the games are to pick this pattern over the
	// variation's others.
	Weight float64

	Layers [Channels]Layer
}

// Layer is what one channel of this pattern draws with.
func (p Pattern) Layer(channel int) Layer {
	if channel < 0 || channel >= Channels {
		return Layer{}
	}

	return p.Layers[channel]
}

// Palette is one way of colouring an accessory: a texture four pixels wide for
// every channel of the mask, and as many rows high as it has shades, one of
// which the games pick at random.
type Palette struct {
	// Weight is how likely the games are to pick this palette over the
	// variation's others.
	Weight float64

	// Texture is the palette, as the game's files write its path.
	Texture string
}

// Variation is a named way of colouring an accessory.
type Variation struct {
	Name string

	// Patterns and Palettes are the alternatives the games choose between,
	// each with the weight it is chosen with.
	Patterns []Pattern
	Palettes []Palette
}

// Library is every pattern, layout and variation a game's folders hold, by
// name. A name a mod defines again is the mod's, which is what the game does
// with the file it replaces.
type Library struct {
	textures   map[string]Textures
	layouts    map[string]Layout
	variations map[string]Variation
}

// Files are the folders a library is read from: a folders.Set of a game and
// its mods is one, and so is a plain folder of files taken out of a game.
type Files interface {
	// Files lists the files of a folder below the folders.
	Files(folder folders.Folder) ([]folders.File, report.Diagnostics)
}

// Read reads the accessory variations of a game and its mods. A file that
// cannot be read or parsed is reported and leaves out what it held.
func Read(set Files, collector *report.Collector) *Library {
	files, _ := set.Files(folders.Folder{Path: variationFolder, Extension: ".txt", Recursive: true})

	library := &Library{
		textures:   map[string]Textures{},
		layouts:    map[string]Layout{},
		variations: map[string]Variation{},
	}

	for _, parsed := range database.ParseFiles(files, collector) {
		library.add(parsed.Document)
	}

	return library
}

// Variation returns a variation of the game by name, the last of that name
// winning, which is the one of the mod that replaced the file.
func (l *Library) Variation(name string) (Variation, bool) {
	if l == nil {
		return Variation{}, false
	}

	found, ok := l.variations[name]

	return found, ok
}

// Textures returns the pattern textures of a name.
func (l *Library) Textures(name string) (Textures, bool) {
	if l == nil {
		return Textures{}, false
	}

	found, ok := l.textures[name]

	return found, ok
}

// Layout returns the layout of a name.
func (l *Library) Layout(name string) (Layout, bool) {
	if l == nil {
		return Layout{}, false
	}

	found, ok := l.layouts[name]

	return found, ok
}

// Empty reports whether nothing at all was read, which is what a game that
// knows no accessory variations gives.
func (l *Library) Empty() bool {
	return l == nil || len(l.variations) == 0 && len(l.textures) == 0 && len(l.layouts) == 0
}

// add reads the definitions of one file of a game or a mod.
func (l *Library) add(document *script.Document) {
	if document == nil {
		return
	}

	for _, node := range document.All("pattern_textures") {
		if read, ok := readTextures(node); ok {
			l.textures[read.Name] = read
		}
	}

	for _, node := range document.All("pattern_layout") {
		if read, ok := readLayout(node); ok {
			l.layouts[read.Name] = read
		}
	}

	for _, node := range document.All("variation") {
		if read, ok := readVariation(node); ok {
			l.variations[read.Name] = read
		}
	}
}

func readTextures(node script.Node) (Textures, bool) {
	name, ok := text(node, "name")
	if !ok {
		return Textures{}, false
	}

	return Textures{
		Name:       name,
		ColourMask: textOrNothing(node, "colormask"),
		Normal:     textOrNothing(node, "normal"),
		Properties: textOrNothing(node, "properties"),
	}, true
}

func readLayout(node script.Node) (Layout, bool) {
	name, ok := text(node, "name")
	if !ok {
		return Layout{}, false
	}

	layout := Layout{Name: name}

	layout.Scale = readRange(node, "scale")
	layout.Rotation = readRange(node, "rotation")

	if offset, ok := node.Get("offset"); ok {
		layout.OffsetX = readRange(offset, "x")
		layout.OffsetY = readRange(offset, "y")
	}

	return layout, true
}

// readRange reads a value that is either a number or the range a number is
// picked from, as the games write both.
func readRange(node script.Node, key string) Range {
	value, ok := node.Get(key)
	if !ok {
		return Range{}
	}

	if number, ok := value.Num(); ok {
		return Range{Min: number, Max: number}
	}

	return Range{Min: number(value, "min"), Max: number(value, "max")}
}

func readVariation(node script.Node) (Variation, bool) {
	name, ok := text(node, "name")
	if !ok {
		return Variation{}, false
	}

	variation := Variation{Name: name}

	for _, pattern := range node.All("pattern") {
		variation.Patterns = append(variation.Patterns, readPattern(pattern))
	}

	for _, palette := range node.All("color_palette") {
		if path, ok := text(palette, "texture"); ok {
			variation.Palettes = append(variation.Palettes, Palette{Weight: weight(palette), Texture: path})
		}
	}

	return variation, true
}

func readPattern(node script.Node) Pattern {
	read := Pattern{Weight: weight(node)}

	for channel, key := range [Channels]string{"r", "g", "b", "a"} {
		layer, ok := node.Get(key)
		if !ok {
			continue
		}

		read.Layers[channel] = Layer{
			Textures: textOrNothing(layer, "textures"),
			Layout:   textOrNothing(layer, "layout"),
		}
	}

	return read
}

// weight is how likely a choice is to be picked, which the games leave at
// nothing when they write none.
func weight(node script.Node) float64 {
	return number(node, "weight")
}

func text(node script.Node, key string) (string, bool) {
	value, ok := node.Get(key)
	if !ok {
		return "", false
	}

	found, ok := value.Str()
	if !ok {
		return "", false
	}

	return found, true
}

func textOrNothing(node script.Node, key string) string {
	found, _ := text(node, key)

	return strings.TrimSpace(found)
}

func number(node script.Node, key string) float64 {
	value, ok := node.Get(key)
	if !ok {
		return 0
	}

	found, _ := value.Num()

	return found
}
