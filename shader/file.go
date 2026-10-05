package shader

import (
	"fmt"
	"strconv"
	"strings"
)

// Stage is the part of the pipeline a declaration belongs to.
type Stage int

const (
	// StageBoth is a declaration outside VertexShader and PixelShader, which
	// both see.
	StageBoth Stage = iota
	StageVertex
	StagePixel
)

// File is a shader file: the .shader files the assets name, and the .fxh
// files they include.
type File struct {
	Path string

	// Includes are the files this one includes, below gfx/FX.
	Includes []string

	// Options are the defines the engine sets on a mesh by what it holds,
	// such as PDX_MESH_UV1 for a mesh with a second set of texture
	// coordinates.
	Options []string

	// Items are the declarations of the file, in the order written.
	Items []Item

	Effects map[string]*Effect

	// States are the blend, depth stencil and rasterizer states by name.
	States map[string]*State
}

// Item is one declaration of a file. Exactly one of its fields but Stage is
// set.
type Item struct {
	Stage Stage

	Code          string
	Declaration   string
	Struct        *Struct
	Buffer        *Buffer
	Sampler       *Sampler
	BufferTexture *BufferTexture
	Main          *Main
}

// Struct is a VertexStruct: a structure whose members carry the semantics
// that tie them to vertex streams, to what passes from the vertex to the
// pixel stage, and to render targets. Its body is kept as written, since
// which members it has depends on the defines.
type Struct struct {
	Name string
	Raw  string
}

// Buffer is a ConstantBuffer: values the engine sets for a draw. Its body is
// kept as written.
type Buffer struct {
	Name string
	Raw  string
}

// Sampler is a TextureSampler.
type Sampler struct {
	Name string

	// Ref is what the engine binds to the sampler: PdxTexture0 and on for
	// the textures of the asset, or a name of its own, such as
	// JominiEnvironmentMap.
	Ref string

	// Index is the slot of the asset's textures the sampler takes, when the
	// file says.
	Index    int
	HasIndex bool

	// File is the texture the sampler takes when nothing else is bound.
	File string
	SRGB bool

	// Type is Cube, 3D or 2darray for a sampler of that kind, and empty for
	// a plain two dimensional one. Compare is set for a sampler that
	// compares, such as the one of a shadow map.
	Type    string
	Compare bool
}

// BufferTexture is a buffer of values the engine binds, read by index.
type BufferTexture struct {
	Name string
	Ref  string
	Type string
}

// Main is a MainCode: the entry point of a stage.
type Main struct {
	Name string

	// Input and Output are the structs the entry point takes and returns.
	// Output is PDX_COLOR for a pixel shader that returns a colour.
	Input  string
	Output string

	Code string

	// Samplers and Buffers are samplers and constant buffers the entry point
	// declares for itself.
	Samplers []*Sampler
	Buffers  []*Buffer
}

// Effect is what an asset names in its shader field: the entry points of
// both stages, the defines they are built with, and the states they are drawn
// with.
type Effect struct {
	Name string

	VertexShader string
	PixelShader  string
	Defines      []string

	BlendState        string
	DepthStencilState string
	RasterizerState   string
}

// State is a BlendState, DepthStencilState or RasterizerState.
type State struct {
	Kind   string
	Name   string
	Values map[string]string
}

// ParseFile reads a shader file. Path is where it is below the game's root,
// such as gfx/FX/pdxmesh.shader.
func ParseFile(path, source string) (*File, error) {
	nodes, err := parseNodes(source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	file := &File{Path: path, Effects: map[string]*Effect{}, States: map[string]*State{}}

	for _, node := range nodes {
		if err := file.add(node, StageBoth); err != nil {
			return nil, fmt.Errorf("%s: line %d: %w", path, node.Line, err)
		}
	}

	return file, nil
}

func (f *File) add(node *Node, stage Stage) error {
	switch strings.ToLower(node.Key) {
	case "includes":
		f.Includes = append(f.Includes, node.Items...)

	case "supports_additional_shader_options":
		f.Options = append(f.Options, node.Items...)

	case "vertexshader", "pixelshader":
		inner := StageVertex
		if strings.EqualFold(node.Key, "PixelShader") {
			inner = StagePixel
		}

		for _, child := range node.Body {
			if err := f.add(child, inner); err != nil {
				return err
			}
		}

	case "code":
		f.Items = append(f.Items, Item{Stage: stage, Code: node.Raw})

	case "vertexstruct":
		f.Items = append(f.Items, Item{Stage: stage, Struct: &Struct{Name: node.Name, Raw: node.Raw}})

	case "struct":
		// A plain struct written outside a code block, such as the ones
		// sharedconstants.fxh declares its constant buffer with, is code
		// in its place. Its # comments are the file's, not the code's.
		f.Items = append(f.Items, Item{Stage: stage, Code: fmt.Sprintf("struct %s\n{\n%s};\n", node.Name, withoutComments(node.Raw))})

	case "sampler":
		// A sampler on its own, for code that samples textures through
		// samplers it picks.
		kind := "SamplerState"
		if field, ok := find(node.Body, "SamplerType"); ok && strings.EqualFold(field.Value, "Compare") {
			kind = "SamplerComparisonState"
		}

		f.Items = append(f.Items, Item{Stage: stage, Declaration: fmt.Sprintf("%s %s;\n", kind, node.Name)})

	case "texture":
		// A texture on its own, sampled through samplers declared apart:
		// of a kind, with a format for one of whole numbers, and an array
		// of textures with ResourceArraySize.
		kind := "Texture2D"
		if field, ok := find(node.Body, "Type"); ok {
			switch strings.ToLower(field.Value) {
			case "cube":
				kind = "TextureCube"
			case "2darray":
				kind = "Texture2DArray"
			case "3d":
				kind = "Texture3D"
			}
		}

		if field, ok := find(node.Body, "format"); ok && field.Value != "" {
			kind += "<" + field.Value + ">"
		}

		array := ""
		if field, ok := find(node.Body, "ResourceArraySize"); ok && field.Value != "" {
			array = "[" + field.Value + "]"
		}

		f.Items = append(f.Items, Item{Stage: stage, Declaration: fmt.Sprintf("%s %s%s;\n", kind, node.Name, array)})

	case "constantbuffer":
		f.Items = append(f.Items, Item{Stage: stage, Buffer: &Buffer{Name: node.Args, Raw: node.Raw}})

	case "texturesampler":
		f.Items = append(f.Items, Item{Stage: stage, Sampler: newSampler(node)})

	case "buffertexture":
		texture := &BufferTexture{Name: node.Name}

		for _, field := range node.Body {
			switch strings.ToLower(field.Key) {
			case "ref":
				texture.Ref = field.Value
			case "type":
				texture.Type = strings.TrimSuffix(field.Value, ";")
			}
		}

		f.Items = append(f.Items, Item{Stage: stage, BufferTexture: texture})

	case "maincode":
		main := &Main{Name: node.Name}

		for _, field := range node.Body {
			switch strings.ToLower(field.Key) {
			case "input":
				main.Input = field.Value
			case "output":
				main.Output = field.Value
			case "code":
				main.Code = field.Raw
			case "texturesampler":
				// A sampler only this entry point reads, such as the
				// alpha of a world decal.
				main.Samplers = append(main.Samplers, newSampler(field))
			case "constantbuffer":
				main.Buffers = append(main.Buffers, &Buffer{Name: field.Args, Raw: field.Raw})
			}
		}

		f.Items = append(f.Items, Item{Stage: stage, Main: main})

	case "effect":
		f.Effects[node.Name] = newEffect(node)

	case "blendstate", "depthstencilstate", "rasterizerstate":
		state := &State{Kind: node.Key, Name: node.Name, Values: map[string]string{}}

		for _, field := range node.Body {
			state.Values[field.Key] = field.Value
		}

		f.States[node.Name] = state
	}

	// Compute shaders and the buffers only they read have no part in drawing
	// a mesh, and are left out.
	return nil
}

// find returns the field of a block with a key, whatever its case.
func find(body []*Node, key string) (*Node, bool) {
	for _, field := range body {
		if strings.EqualFold(field.Key, key) {
			return field, true
		}
	}

	return nil, false
}

// withoutComments drops the # comments of a body of declarations, keeping
// the preprocessor directives.
func withoutComments(raw string) string {
	var out strings.Builder

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") && !isDirective(trimmed) {
			continue
		}

		if comment := strings.Index(line, "#"); comment >= 0 && !strings.HasPrefix(trimmed, "#") {
			line = line[:comment]
		}

		out.WriteString(line)
		out.WriteString("\n")
	}

	return out.String()
}

func newSampler(node *Node) *Sampler {
	sampler := &Sampler{Name: node.Name}

	for _, field := range node.Body {
		switch strings.ToLower(field.Key) {
		case "ref":
			sampler.Ref = field.Value
		case "index":
			if index, err := strconv.Atoi(field.Value); err == nil {
				sampler.Index, sampler.HasIndex = index, true
			}
		case "file":
			sampler.File = field.Value
		case "srgb":
			sampler.SRGB = strings.EqualFold(field.Value, "yes")
		case "type":
			sampler.Type = field.Value
		case "samplertype":
			sampler.Compare = strings.EqualFold(field.Value, "Compare")
		}
	}

	return sampler
}

func newEffect(node *Node) *Effect {
	effect := &Effect{Name: node.Name}

	for _, field := range node.Body {
		switch strings.ToLower(field.Key) {
		case "vertexshader":
			effect.VertexShader = field.Value
		case "pixelshader":
			effect.PixelShader = field.Value
		case "defines":
			effect.Defines = append(effect.Defines, field.Items...)
		case "blendstate":
			effect.BlendState = field.Value
		case "depthstencilstate":
			effect.DepthStencilState = field.Value
		case "rasterizerstate":
			effect.RasterizerState = field.Value
		}
	}

	return effect
}
