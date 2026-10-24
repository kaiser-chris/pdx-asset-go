package render

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// Decals are textures the genes of a portrait lay over its skin: wrinkles,
// stubble, the colour of the lips, the eyebrows. The games lay them in their
// skin shader, for every pixel drawn, out of arrays of textures with the
// decals' numbers in a buffer; how many a head carries has no limit there.
//
// A shader of raylib's has a handful of texture units to bind them to, so
// here they are laid over the maps of a part instead, on the GPU, whenever
// they change: one pass of the shader of decals for each decal and map it
// has, into a target of half floats, and a last pass back into a map like
// the part's own, which the part is then drawn with. The decals change when
// the genes or the age do, which is seldom next to how often a portrait is
// drawn.

// BlendMode is how a map of a decal is laid over the skin's, as the games'
// gene files name them and their shader numbers them.
type BlendMode int

const (
	// BlendOverlay darkens what is dark of the skin and lightens what is
	// light by the decal; for a normal map it adds the slopes of the
	// decal's to the skin's.
	BlendOverlay BlendMode = 0

	// BlendReplace puts the decal in the place of the skin.
	BlendReplace BlendMode = 1

	// BlendHardLight darkens or lightens the skin by what is dark or light
	// of the decal.
	BlendHardLight BlendMode = 2

	// BlendMultiply multiplies the skin by the decal.
	BlendMultiply BlendMode = 3
)

// blendModes are the blend modes by the names the gene files give them.
var blendModes = map[string]BlendMode{
	"overlay":    BlendOverlay,
	"replace":    BlendReplace,
	"hard_light": BlendHardLight,
	"multiply":   BlendMultiply,
}

// ParseBlendMode returns the blend mode a gene file names, such as multiply.
// It reports false for a name it does not know, and for none, which the
// games take as an overlay, the first of their blend modes.
func ParseBlendMode(name string) (BlendMode, bool) {
	mode, ok := blendModes[name]

	return mode, ok
}

// Decal is a texture laid over the skin of the parts of a portrait that are
// one body part, such as the head.
type Decal struct {
	// BodyPart is the part of a portrait the decal is laid over, as the
	// entities' game data names them; see model.Part.DecalBodyPart.
	BodyPart string

	// The maps of the decal, each nil where it has none, and how each is
	// laid over the part's own. The diffuse map's alpha makes the decal
	// less opaque wherever it is, for all of its maps.
	Diffuse, Normal, Properties             *texture.Image
	DiffuseMode, NormalMode, PropertiesMode BlendMode

	// Weight is how opaque the decal is, from 0 to 1, as the strength of its
	// gene and the age of the portrait make it.
	Weight float64

	// Priority orders the decals: one of a higher priority is laid over one
	// of a lower. Decals of the same priority are laid in the order given.
	Priority int

	// PostSkinColor lays the decal after the skin colour is blended in,
	// rather than before, where it is tinted by it.
	PostSkinColor bool
}

// The maps decals are laid over: the diffuse map, the normal map and the
// properties map, as the shader of decals numbers their kinds, and the maps
// of a raylib material each is.
const decalMaps = 3

var decalMaterialMaps = [decalMaps]int32{rl.MapDiffuse, rl.MapNormal, rl.MapSpecular}

// mapOf is the map of a decal of a kind, and how it is laid.
func (d *Decal) mapOf(kind int) (*texture.Image, BlendMode) {
	switch kind {
	case 0:
		return d.Diffuse, d.DiffuseMode
	case 1:
		return d.Normal, d.NormalMode
	}

	return d.Properties, d.PropertiesMode
}

// The stages of a pass of the shader of decals; see shaders/decal.fs.
const (
	stageRead = iota
	stageLay
	stageTurn
	stagePack
)

// decalShader is the shader the decals are laid with, and the places of its
// uniforms.
type decalShader struct {
	shader rl.Shader

	laid, own, decalDiffuse, decalMap int32
	size, stage, kind                 int32
	hasDiffuse, hasMap, mode, weight  int32
	paletteColor, colorMaskInterval   int32
}

// loadDecalShader compiles the shader decals are laid with. raylib's own
// vertex shader draws the rectangle each pass covers its target with.
func loadDecalShader() (decalShader, error) {
	fragment, err := shaders.ReadFile("shaders/decal.fs")
	if err != nil {
		return decalShader{}, err
	}

	shader := rl.LoadShaderFromMemory("", string(fragment))
	if shader.ID == 0 || shader.ID == rl.GetShaderIdDefault() {
		return decalShader{}, fmt.Errorf("the decal shader did not compile")
	}

	location := func(name string) int32 {
		return rl.GetShaderLocation(shader, name)
	}

	return decalShader{
		shader:            shader,
		laid:              location("laid"),
		own:               location("own"),
		decalDiffuse:      location("decalDiffuse"),
		decalMap:          location("decalMap"),
		size:              location("size"),
		stage:             location("stage"),
		kind:              location("kind"),
		hasDiffuse:        location("hasDiffuse"),
		hasMap:            location("hasMap"),
		mode:              location("mode"),
		weight:            location("weight"),
		paletteColor:      location("paletteColor"),
		colorMaskInterval: location("colorMaskInterval"),
	}, nil
}

// target is a texture that can be drawn into.
type target struct {
	framebuffer   uint32
	texture       rl.Texture2D
	width, height int32
}

// newTarget makes a texture of a size and format that can be drawn into.
// One of RGBA8 has the smaller copies of itself a map is drawn with from
// afar, made again whenever it is drawn into.
func newTarget(width, height int32, format texture.Format) (target, error) {
	made, err := UploadTexture(&texture.Image{Width: int(width), Height: int(height), Format: format, Levels: 1})
	if err != nil {
		return target{}, err
	}

	framebuffer := rl.LoadFramebuffer()
	rl.FramebufferAttach(framebuffer, made.ID, rl.AttachmentColorChannel0, rl.AttachmentTexture2d, 0)

	if framebuffer == 0 || !rl.FramebufferComplete(framebuffer) {
		if framebuffer != 0 {
			rl.UnloadFramebuffer(framebuffer)
		}

		rl.UnloadTexture(made)

		return target{}, fmt.Errorf("the GPU cannot draw into a %v texture of %dx%d", format, width, height)
	}

	return target{framebuffer: framebuffer, texture: made, width: width, height: height}, nil
}

func (t *target) unload() {
	if t.framebuffer != 0 {
		rl.UnloadFramebuffer(t.framebuffer)
	}

	if t.texture.ID != 0 {
		rl.UnloadTexture(t.texture)
	}

	*t = target{}
}

// scratch are the two targets of half floats decals are laid between, one
// read while the other is drawn, for maps of a size. The renderer keeps them
// for every model it lays decals for.
type scratch [2]target

// scratchOf returns the targets decals are laid between for maps of a size,
// made the first time a map of that size is laid over.
func (r *Renderer) scratchOf(width, height int32) (*scratch, error) {
	key := [2]int32{width, height}

	if found, ok := r.scratch[key]; ok {
		return found, nil
	}

	made := &scratch{}

	for index := range made {
		one, err := newTarget(width, height, texture.RGBA16F)
		if err != nil {
			for _, done := range made[:index] {
				done.unload()
			}

			return nil, err
		}

		made[index] = one
	}

	if r.scratch == nil {
		r.scratch = map[[2]int32]*scratch{}
	}

	r.scratch[key] = made

	return made, nil
}

// unloadDecals releases the shader of decals and the targets they are laid
// between.
func (r *Renderer) unloadDecals() {
	for _, pair := range r.scratch {
		for index := range pair {
			pair[index].unload()
		}
	}

	r.scratch = nil

	if r.decals.shader.ID != 0 {
		rl.UnloadShader(r.decals.shader)
		r.decals = decalShader{}
	}
}

// partDecals are the decals laid over a part: the maps they were laid into,
// which the part is drawn with, and the decals laid, so that the same ones
// are not laid again. Where no decal is laid over a map, the part is drawn
// with its base map again.
type partDecals struct {
	laid [decalMaps]target

	drawn []Decal
	look  Look

	// paletted is set when the skin colour is blended into the laid diffuse
	// map, for the decals laid after it, so that the part is drawn without
	// blending it in again.
	paletted bool
}

// LayDecals lays decals over the skin of the model's parts: each over the
// parts of the body part it names, such as the head, whose effect is one of
// the games' skin effects, as only their skin shader lays decals. The look
// gives the skin colour, which decals laid after it are laid over; it is
// only used for those. A part that no decal is laid over is drawn with its
// own maps again, which an empty list does for all of them.
//
// The decals are laid in order of priority, those laid before the skin
// colour first, as the games sort them, and those of no weight are left
// out. The textures of the decals are handed to the GPU once and kept for as
// long as the model lives.
//
// It has to run on the thread that owns the OpenGL context, after the model
// has been uploaded, and outside the drawing of a viewer.
func (m *Model) LayDecals(decals []Decal, look Look) error {
	sorted := make([]Decal, 0, len(decals))

	for _, decal := range decals {
		if decal.Weight > 0 && decal.BodyPart != "" {
			sorted = append(sorted, decal)
		}
	}

	slices.SortStableFunc(sorted, func(a, b Decal) int {
		if a.PostSkinColor != b.PostSkinColor {
			if a.PostSkinColor {
				return 1
			}

			return -1
		}

		return cmp.Compare(a.Priority, b.Priority)
	})

	// Only skin takes decals: the games lay them in their PS_skin alone. Europa
	// Universalis 5 names the hair a part of the head as well, which its
	// PS_hair draws without them, coloured by the hair colour alone.
	for index := range m.parts {
		part := &m.parts[index]
		if part.decalBodyPart == "" || !part.look.palette || part.look.paletteKind != paletteSkin {
			continue
		}

		var own []Decal

		for _, decal := range sorted {
			if decal.BodyPart == part.decalBodyPart {
				own = append(own, decal)
			}
		}

		if err := m.layPart(part, own, look); err != nil {
			return fmt.Errorf("model %s: %w", m.Name, err)
		}
	}

	return nil
}

// layPart lays the decals of one part, in order, over each of its maps.
func (m *Model) layPart(part *gpuPart, decals []Decal, look Look) error {
	if part.decals == nil {
		if len(decals) == 0 {
			return nil
		}

		part.decals = &partDecals{}
	}

	laid := part.decals

	if slices.Equal(decals, laid.drawn) && (!postSkinColor(decals) || look == laid.look) {
		return nil
	}

	laid.drawn = slices.Clone(decals)
	laid.look = look
	laid.paletted = false

	for kind, slot := range decalMaterialMaps {
		drawn, err := m.layMap(part, kind, decals, look)
		if err != nil {
			return err
		}

		if !drawn {
			rl.SetMaterialTexture(part.material, slot, part.baseMaps[kind])

			continue
		}

		rl.SetMaterialTexture(part.material, slot, laid.laid[kind].texture)
	}

	return nil
}

// postSkinColor reports whether any of decals is laid after the skin colour.
func postSkinColor(decals []Decal) bool {
	return slices.ContainsFunc(decals, func(decal Decal) bool { return decal.PostSkinColor })
}

// layMap lays the decals that have a map of a kind over the part's map of
// that kind, and reports whether there were any. The map is laid at the size
// of the larger of the part's own map and the decals' maps, so that a part
// whose map is a stand in of one pixel is laid at the decals' size.
func (m *Model) layMap(part *gpuPart, kind int, decals []Decal, look Look) (bool, error) {
	laid := part.decals
	own := part.baseMaps[kind]

	var (
		uploaded []rl.Texture2D
		diffuses []rl.Texture2D
		used     []Decal
		after    bool
		width    = own.Width
		height   = own.Height
	)

	for _, decal := range decals {
		image, _ := decal.mapOf(kind)
		if image == nil {
			continue
		}

		done, err := m.texture(image, m.renderer.white)
		if err != nil {
			return false, err
		}

		diffuse, err := m.texture(decal.Diffuse, m.renderer.white)
		if err != nil {
			return false, err
		}

		uploaded = append(uploaded, done)
		diffuses = append(diffuses, diffuse)
		used = append(used, decal)
		after = after || decal.PostSkinColor
		width, height = max(width, done.Width), max(height, done.Height)
	}

	if len(used) == 0 {
		return false, nil
	}

	// The skin colour is blended into the diffuse map here only for the
	// decals laid after it; otherwise the part's shader blends it in, as it
	// does without decals.
	if kind == 0 && after {
		laid.paletted = true
	}

	if laid.laid[kind].width != width || laid.laid[kind].height != height {
		laid.laid[kind].unload()

		made, err := newTarget(width, height, texture.RGBA8)
		if err != nil {
			return false, err
		}

		laid.laid[kind] = made
	}

	pair, err := m.renderer.scratchOf(width, height)
	if err != nil {
		return false, err
	}

	r := m.renderer
	shader := &r.decals
	current := 0

	pass := func(into target, stage int, set func()) {
		rl.BeginTextureMode(rl.RenderTexture2D{ID: into.framebuffer, Texture: into.texture})
		rl.BeginShaderMode(shader.shader)
		rl.DisableColorBlend()

		rl.SetShaderValue(shader.shader, shader.size, []float32{float32(width), float32(height)}, rl.ShaderUniformVec2)
		rl.SetShaderValue(shader.shader, shader.stage, []float32{float32(stage)}, rl.ShaderUniformFloat)
		rl.SetShaderValue(shader.shader, shader.kind, []float32{float32(kind)}, rl.ShaderUniformFloat)

		set()

		rl.DrawRectangle(0, 0, width, height, rl.White)

		rl.EndShaderMode()
		rl.EnableColorBlend()
		rl.EndTextureMode()
	}

	// Every pass binds all four of its textures, so that no sampler is left
	// bound to whatever a pass before it used.
	bind := func(laidTexture, ownTexture, decalDiffuse, decalMap rl.Texture2D) {
		rl.SetShaderValueTexture(shader.shader, shader.laid, laidTexture)
		rl.SetShaderValueTexture(shader.shader, shader.own, ownTexture)
		rl.SetShaderValueTexture(shader.shader, shader.decalDiffuse, decalDiffuse)
		rl.SetShaderValueTexture(shader.shader, shader.decalMap, decalMap)
	}

	pass(pair[current], stageRead, func() {
		bind(r.white, own, r.white, r.white)
	})

	turned := false

	for index, decal := range used {
		if decal.PostSkinColor && !turned {
			turned = true

			pass(pair[1-current], stageTurn, func() {
				bind(pair[current].texture, own, r.white, r.white)
				setLook(shader, look, colorMaskInterval(part, look.ColorMaskInterval))
			})

			current = 1 - current
		}

		_, mode := decal.mapOf(kind)

		pass(pair[1-current], stageLay, func() {
			bind(pair[current].texture, own, diffuses[index], uploaded[index])

			rl.SetShaderValue(shader.shader, shader.hasDiffuse, []float32{boolFloat(decal.Diffuse != nil)}, rl.ShaderUniformFloat)
			rl.SetShaderValue(shader.shader, shader.hasMap, []float32{1}, rl.ShaderUniformFloat)
			rl.SetShaderValue(shader.shader, shader.mode, []float32{float32(mode)}, rl.ShaderUniformFloat)
			rl.SetShaderValue(shader.shader, shader.weight, []float32{float32(decal.Weight)}, rl.ShaderUniformFloat)
		})

		current = 1 - current
	}

	pass(laid.laid[kind], stagePack, func() {
		bind(pair[current].texture, own, r.white, r.white)
	})

	rl.GenTextureMipmaps(&laid.laid[kind].texture)
	rl.SetTextureFilter(laid.laid[kind].texture, rl.FilterTrilinear)

	return true, nil
}

// setLook hands the skin colour and the interval of the part's colour mask
// to the shader of decals, the colour in linear light: a Look holds it as the
// renderer's shader blends it, in sRGB; see PaletteColour.
func setLook(shader *decalShader, look Look, interval [2]float32) {
	var linear [3]float32

	for channel, value := range look.PaletteColor {
		linear[channel] = float32(math.Pow(float64(value), gamma))
	}

	rl.SetShaderValue(shader.shader, shader.paletteColor, linear[:], rl.ShaderUniformVec3)
	rl.SetShaderValue(shader.shader, shader.colorMaskInterval, interval[:], rl.ShaderUniformVec2)
}

// unloadDecals releases the maps decals were laid into, and leaves the part
// to be drawn with its own.
func (p *gpuPart) unloadDecals() {
	if p.decals == nil {
		return
	}

	for kind := range p.decals.laid {
		p.decals.laid[kind].unload()
	}

	p.decals = nil
}
