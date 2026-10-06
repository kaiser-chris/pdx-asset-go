package render

import (
	"path"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-parser-go/script"

	"github.com/kaiser-chris/pdx-asset-go/shader"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// What a sampler reads that neither the asset nor the sampler gives a
// texture: what the engine would bind for a model on the map, standing in
// for a model shown on its own, off the map and without any state of a game.
//
// White, the most basic texture, is right for most: it leaves what it is
// multiplied with as it is. It is wrong for what the engine binds to lay the
// state of the game over a model, which the shaders blend in by its alpha or
// by a mask, and which white lays over everything: the colour and highlight
// of a province, devastation, winter, the pattern of a garment. Those read
// nothing, transparent black, as a model without that state does.
//
// Victoria 3 names the textures of its map for the engine in
// gfx/map/textures/maptextures.settings, such as the colour map its trees
// are tinted with; a sampler of one of those reads the game's own file. Those
// the shaders read at a model's place on the map read its average colour,
// since the model shown on its own stands nowhere on it.

// noneRefs are what the engine binds the state of a game to, and noneNames
// the samplers that read such a state through a reference the engine binds
// other things to as well, such as the mask of a garment's patterns.
var (
	noneRefs = map[string]bool{
		"JominiProvinceColor":            true,
		"JominiProvinceColorIndirection": true,
		"JominiBorderDistance":           true,
		"DevastationPollutionMask":       true,
		"WinterTexture":                  true,
		"PdxMeshDecalsTexture":           true,
	}

	noneNames = map[string]bool{
		"PatternMask":    true,
		"CoaPatternMask": true,
	}
)

// mapTexturesFile is where Victoria 3 names the textures of its map.
const mapTexturesFile = "gfx/map/textures/maptextures.settings"

// atMapPlace are the textures of maptextures.settings the shaders read at a
// model's place on the map, by their name there, in lower case.
var atMapPlace = map[string]bool{
	"colormaptree":      true,
	"windmaptree":       true,
	"flatmap":           true,
	"flatmapoverlay":    true,
	"impassableterrain": true,
}

// standIn is what a sampler reads that nothing else gives a texture, and
// whether it is converted from sRGB as it is sampled. The textures of the map
// hold colours, and the shaders treat what they sample of them as linear:
// tree.shader converts its colour map back to gamma before blending by it.

// dataMapTextures are the textures of maptextures.settings that hold data
// rather than colours, by their name there in lower case: the directions of
// the wind trees sway in.
var dataMapTextures = map[string]bool{
	"windmaptree": true,
}

type standIn struct {
	texture rl.Texture2D
	srgb    bool
}

// The colour maps the games tint what stands on the map with, by its
// place: the decals of the ground by the colour of the terrain, the trees by
// a map of their own. The model editor shows a model on no map, untinted:
// Victoria 3's southern_plantation_entity looks there as its decal's diffuse
// map does. Every shader blends its colour map so that a mid grey of 0.5
// leaves what it tints as it is (Crusader Kings 3's trees use that grey
// themselves where they cannot read the map), each reading the map its way:
// Victoria 3's decals blend by what they read of the terrain's as it is,
// soft light, and its trees by what they read of theirs, converted to sRGB
// first, overlay. So both stand in as a grey of 128, read as its sampler
// reads the map: the terrain's as it is, which Crusader Kings 3 converts
// from sRGB itself, the trees' converted from sRGB.
var tintMaps = map[string]bool{
	"pdxterraincolormap": false,
	"colormaptree":       true,
}

// standIn is what a sampler reads that nothing else gives a texture.
func (r *Renderer) standIn(bound shader.Texture) standIn {
	sampler := bound.Sampler
	if sampler == nil {
		return standIn{texture: r.white}
	}

	if noneRefs[sampler.Ref] || noneNames[sampler.Name] {
		return standIn{texture: r.none}
	}

	ref := strings.ToLower(sampler.Ref)
	if srgb, ok := tintMaps[ref]; ok {
		return standIn{texture: r.grey, srgb: srgb}
	}

	if name, ok := r.mapTextures()[ref]; ok {
		return standIn{texture: r.mapTexture(name, atMapPlace[ref]), srgb: !dataMapTextures[ref]}
	}

	return standIn{texture: r.white}
}

// mapTextures reads maptextures.settings once: the files of the map's
// textures by their name, in lower case, which the references of the
// samplers spell in their own case.
func (r *Renderer) mapTextures() map[string]string {
	if r.mapFiles != nil {
		return r.mapFiles
	}

	r.mapFiles = map[string]string{}

	if r.source == nil {
		return r.mapFiles
	}

	data, err := r.source.ReadFile(mapTexturesFile)
	if err != nil {
		return r.mapFiles
	}

	for _, field := range script.Parse(string(data)).Fields {
		if file, ok := field.Value.Str(); ok && file != "" {
			r.mapFiles[strings.ToLower(field.Key)] = path.Join(path.Dir(mapTexturesFile), file)
		}
	}

	return r.mapFiles
}

// mapTexture uploads a texture of the map once, or its average colour. One
// that cannot be read is white.
func (r *Renderer) mapTexture(name string, average bool) rl.Texture2D {
	key := "map:" + name
	if average {
		key = "average:" + name
	}

	if found, ok := r.files[key]; ok {
		return found
	}

	if r.files == nil {
		r.files = map[string]rl.Texture2D{}
	}

	uploaded := r.white

	if data, err := r.source.ReadFile(name); err == nil {
		if decoded, err := texture.Decode(name, data); err == nil {
			if done, err := UploadTexture(decoded); err == nil {
				uploaded = done

				if average {
					uploaded = averageColor(done)
					rl.UnloadTexture(done)
				}
			}
		}
	}

	r.files[key] = uploaded

	return uploaded
}

// averageColor draws a texture into a single pixel through its smallest
// level, which is the average of its colours, and returns that pixel.
func averageColor(source rl.Texture2D) rl.Texture2D {
	if source.Mipmaps <= 1 {
		rl.GenTextureMipmaps(&source)
	}

	rl.SetTextureFilter(source, rl.FilterTrilinear)

	target := rl.LoadRenderTexture(1, 1)

	rl.BeginTextureMode(target)
	rl.ClearBackground(rl.Blank)
	rl.DrawTexturePro(source,
		rl.Rectangle{Width: float32(source.Width), Height: float32(source.Height)},
		rl.Rectangle{Width: 1, Height: 1},
		rl.Vector2{}, 0, rl.White)
	rl.EndTextureMode()

	pixel := rl.LoadImageFromTexture(target.Texture)
	rl.UnloadRenderTexture(target)

	averaged := rl.LoadTextureFromImage(pixel)
	rl.UnloadImage(pixel)

	return averaged
}
