package entity

import (
	"path"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/report"
	"github.com/kaiser-chris/pdx-parser-go/script"

	"github.com/kaiser-chris/pdx-asset-go/model"
	"github.com/kaiser-chris/pdx-asset-go/pattern"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// The keys of the game data block a portrait accessory is written in, which
// Victoria 3 and Crusader Kings 3 both use:
//
//	game_data = {
//		portrait_entity_user_data = {
//			portrait_accessory = { pattern_mask = "..." variation = "..." }
//		}
//	}
const (
	accessoryUserData  = "portrait_entity_user_data"
	accessoryKey       = "portrait_accessory"
	accessoryMask      = "pattern_mask"
	accessoryVariation = "variation"
)

// accessorySource is the accessory an entity's game data names: the mask that
// says where each pattern goes, and the variation that says what they are.
type accessorySource struct {
	Mask      string
	Variation string
}

// readAccessory reads the portrait accessory an entity's own game data names,
// if it names one. The last of a key wins, as it does everywhere in the games'
// files.
func readAccessory(gameData script.Node) (accessorySource, bool) {
	userData, ok := gameData.Get(accessoryUserData)
	if !ok {
		return accessorySource{}, false
	}

	accessory, ok := userData.Get(accessoryKey)
	if !ok {
		return accessorySource{}, false
	}

	source := accessorySource{
		Mask:      text(accessory, accessoryMask),
		Variation: text(accessory, accessoryVariation),
	}

	if source.Mask == "" && source.Variation == "" {
		return accessorySource{}, false
	}

	return source, true
}

func text(node script.Node, key string) string {
	value, ok := node.Get(key)
	if !ok {
		return ""
	}

	found, _ := value.Str()

	return strings.TrimSpace(found)
}

// patterns reads the accessory variations of the game's folders the first time
// an entity asks for one, since models that have no accessory never need them.
func (l *Loader) patterns(collector *report.Collector) *pattern.Library {
	if l.variations == nil {
		l.variations = pattern.Read(l.set, collector)
	}

	return l.variations
}

// accessory reads the accessory an entity's game data names, whole: the mask,
// and every pattern and palette the variation offers to draw it with. What
// cannot be found is reported, and the accessory is drawn without it.
//
// An entity that names an accessory there is not is drawn without one, and the
// entity itself is drawn all the same: a portrait accessory colours a model
// that is already there.
func (l *Loader) accessory(entity *asset.Entity, subject string, collector *report.Collector) *model.Accessory {
	source, ok := readAccessory(entity.GameData)
	if !ok {
		return nil
	}

	origin := entity.Origin()

	note := func(format string, args ...any) {
		collector.Addf(report.SeverityWarning, origin.Source, origin.Path, origin.Line, 0, subject, format, args...)
	}

	if source.Variation == "" {
		note("game data names the pattern mask %s but no variation to colour it with; drawn uncoloured", source.Mask)

		return nil
	}

	variation, ok := l.patterns(collector).Variation(source.Variation)
	if !ok {
		note("game data names the variation %s, which is in none of the gfx/portraits/accessory_variations files; drawn uncoloured", source.Variation)

		return nil
	}

	mask := l.accessoryTexture(source.Mask, subject, collector)
	if mask == nil {
		note("the variation %s has no pattern mask to draw it through; drawn uncoloured", source.Variation)

		return nil
	}

	accessory := &model.Accessory{Mask: mask, Variation: variation.Name}

	for _, choice := range variation.Patterns {
		accessory.Patterns = append(accessory.Patterns, l.accessoryPattern(choice, subject, collector))
	}

	for _, choice := range variation.Palettes {
		accessory.Palettes = append(accessory.Palettes, l.accessoryPalette(choice, subject, collector))
	}

	if len(accessory.Patterns) == 0 || len(accessory.Palettes) == 0 {
		note("the variation %s offers no %s to draw with; drawn uncoloured", variation.Name,
			plural(len(accessory.Patterns) == 0, "pattern", "colour palette"))

		return nil
	}

	return accessory
}

// accessoryPattern reads one way of laying a pattern over an accessory: the
// pattern of each of the mask's channels, and how it is placed.
func (l *Loader) accessoryPattern(choice pattern.Pattern, subject string, collector *report.Collector) model.AccessoryPattern {
	read := model.AccessoryPattern{}
	described := make([]string, 0, pattern.Channels)

	// A channel the pattern says nothing about is left where the surface's
	// own coordinates put it, and draws the colour of the palette alone.
	for channel := range pattern.Channels {
		read.Layers[channel].Placement = pattern.Placement{Scale: 1}
	}

	for channel := range pattern.Channels {
		layer := choice.Layer(channel)

		textures, ok := l.patterns(collector).Textures(layer.Textures)
		if !ok {
			if layer.Textures != "" {
				collector.Addf(report.SeverityWarning, "", "", 0, 0, subject,
					"the pattern %s is in none of the gfx/portraits/accessory_variations files; that channel is drawn in one colour", layer.Textures)
			}

			continue
		}

		described = append(described, textures.Name)

		read.Layers[channel] = model.AccessoryLayer{
			ColourMask: l.accessoryTexture(textures.ColourMask, subject, collector),
			Normal:     l.accessoryTexture(textures.Normal, subject, collector),
			Properties: l.accessoryTexture(textures.Properties, subject, collector),
			Placement:  l.patternPlacement(layer.Layout, collector),
		}
	}

	read.Description = strings.Join(distinct(described), ", ")

	return read
}

// patternPlacement is how a layout places a pattern. A layout that is not
// there places it as the surface's own texture coordinates are.
func (l *Loader) patternPlacement(name string, collector *report.Collector) pattern.Placement {
	if name == "" {
		return pattern.Placement{Scale: 1}
	}

	layout, ok := l.patterns(collector).Layout(name)
	if !ok {
		return pattern.Placement{Scale: 1}
	}

	return layout.Pinned()
}

// accessoryPalette reads one way of colouring an accessory: the colours of its
// palette, which are read out of the texture rather than sampled from it, a
// part having more textures to draw with than a material has places to put
// them.
func (l *Loader) accessoryPalette(choice pattern.Palette, subject string, collector *report.Collector) model.AccessoryPalette {
	image := l.accessoryTexture(choice.Texture, subject, collector)

	colours, read := model.PaletteColours(image)

	if !read && image != nil {
		collector.Addf(report.SeverityWarning, "", "", 0, 0, subject,
			"the colours of the palette %s are not there to be read, being a texture the graphics card would decompress; the accessory is drawn in the colour of its pattern",
			path.Base(strings.ReplaceAll(choice.Texture, "\\", "/")))
	}

	return model.AccessoryPalette{
		Description: path.Base(strings.ReplaceAll(choice.Texture, "\\", "/")),
		Colours:     colours,
		Read:        read,
	}
}

// accessoryTexture finds and decodes a texture the accessory data names.
//
// The games write these paths below the game's root rather than beside the
// file that names them, so the path is looked for as it is written first; a
// mod that writes one beside its own file is found as well.
func (l *Loader) accessoryTexture(name, subject string, collector *report.Collector) *texture.Image {
	if name == "" {
		return nil
	}

	relative := cleanPath(name)

	found, ok := l.set.Find(relative)
	if !ok {
		found, ok = l.set.Find(name)
	}

	if !ok {
		found, ok = l.lookUp(database.Origin{}, name, func(format string, args ...any) {
			collector.Addf(report.SeverityWarning, "", "", 0, 0, subject, format, args...)
		})
	}

	if !ok {
		collector.Addf(report.SeverityWarning, "", "", 0, 0, subject,
			"texture %s is not at %s in any of the folders, nor anywhere below %s; that channel is drawn without its pattern",
			name, relative, lookupFolder)

		return nil
	}

	cached, ok := l.textures[found.Path]
	if !ok {
		cached = decode(found.Path)
		l.textures[found.Path] = cached
	}

	if cached.err != nil {
		collector.Addf(report.SeverityWarning, "", "", 0, 0, subject,
			"texture %s: %v; that channel is drawn without its pattern", name, cached.err)

		return nil
	}

	return cached.image
}

// unique keeps the first of each name, in order, so that a pattern used by
// every channel is described once.
func distinct(names []string) []string {
	var kept []string

	for _, name := range names {
		if !holds(kept, name) {
			kept = append(kept, name)
		}
	}

	return kept
}

func holds(names []string, name string) bool {
	for _, kept := range names {
		if kept == name {
			return true
		}
	}

	return false
}

// cleanPath is a path as the games' files write it: with forward slashes and
// nothing to climb out of the root.
func cleanPath(name string) string {
	return path.Clean(strings.ReplaceAll(name, "\\", "/"))
}

func plural(many bool, one, several string) string {
	if many {
		return several
	}

	return one
}
