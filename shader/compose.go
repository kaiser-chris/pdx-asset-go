package shader

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/kaiser-chris/pdx-asset-go/internal/glslcross"
)

// Source reads shader files by their path below the game's root, such as
// gfx/FX/cw/pdxmesh.fxh.
type Source interface {
	ReadFile(path string) ([]byte, error)
}

// folderFX is the folder the shader files and their includes are in.
const folderFX = "gfx/FX"

// The preludes the engine puts in front of every shader it compiles as HLSL:
// what maps the shared shader language to HLSL, and the definitions every
// platform shares. They are plain code, included by no file.
var preludes = []string{"cw/defines_hlsl.fxh", "cw/defines_common.fxh"}

// engineDefines are the defines the engine sets for every shader it compiles,
// which the files use without defining:
//
//   - PDX_DIRECTX_11 picks the code the engine compiles for its Direct3D 11
//     renderer, one of the renderers the games ship, rather than the code
//     for bindless resources, which only Direct3D 12 and Vulkan have. The
//     prelude also defines variadic macros for every other renderer, which
//     glslang's preprocessor does not read.
//   - PDX_MAX_HEIGHTMAP_COMPRESS_LEVELS is the length of an array of the
//     heightmap. Victoria 3's heightmap.fxh has since written the length out
//     and notes the value of the define: 5.
//   - PDX_MAX_DETAIL_TEXTURES is the length of the array of Crusader Kings
//     3's terrain that holds the tiling of each of its terrain materials, of
//     which its materials.settings defines 115. Its value is nowhere in the
//     files; 256 holds every material.
var engineDefines = []string{
	"PDX_DIRECTX_11",
	"PDX_MAX_HEIGHTMAP_COMPRESS_LEVELS 5",
	"PDX_MAX_DETAIL_TEXTURES 256",
}

// engineCode stands in for what the engine adds to the code itself.
//
// Europa Universalis 5 reads the textures of its terrain materials from the
// engine's table of bindless textures, by a handle, through
// GetBindlessTexture2DNonUniform, which no file defines. Here every handle
// reads the same texture, which the renderer binds white, as it does every
// texture nothing names.
const engineCode = `
Texture2D PdxBindlessTexture2D;
Texture2D GetBindlessTexture2DNonUniform( uint Handle ) { return PdxBindlessTexture2D; }
`

// entryPoint is the function each stage is compiled from, which hands over to
// the entry point the shader file writes.
const entryPoint = "pdx_entry"

// minimumBufferArray is the length a constant buffer's array is declared with
// at least. The engine binds whole buffers of instance data behind arrays the
// files declare short: PdxMeshInstanceData declares float4 Data[2], with the
// comment that a length of 4096 made compiling slow, and the code reads the
// world matrix and the constants of an instance from Data[0] to Data[4].
const minimumBufferArray = 16

// Library reads shader files and builds programs of their effects. It keeps
// every file it has read.
type Library struct {
	source Source
	files  map[string]*File
}

// NewLibrary returns a library reading shader files from a source.
func NewLibrary(source Source) *Library {
	return &Library{source: source, files: map[string]*File{}}
}

// Program is an effect built for OpenGL: the GLSL of both stages, and what
// is bound to them.
type Program struct {
	Effect *Effect

	// Vertex and Pixel are the GLSL of the stages, and VertexHLSL and
	// PixelHLSL the HLSL they were compiled from.
	Vertex, Pixel         string
	VertexHLSL, PixelHLSL string

	// Textures are the samplers of the GLSL, what the program reads
	// textures through, of both stages.
	Textures []Texture

	// BufferTextures are the buffers the stages read, by their name in the
	// GLSL.
	BufferTextures []*BufferTexture

	// Uniforms are the values of the constant buffers both stages read, by
	// the names OpenGL knows them under.
	Uniforms []Uniform

	// Attributes are the inputs of the vertex stage, with the location of
	// the vertex stream each reads.
	Attributes []Attribute

	// States are the blend, depth stencil and rasterizer states of the
	// effect, by kind; one the effect does not name is missing.
	States map[string]*State

	// declared are the samplers the files of the program declare.
	declared []*Sampler
}

// Texture is a sampler of the GLSL: its name and its type, such as sampler2D
// or samplerCube, and the sampler of the shader files it stands for, if it
// stands for one. A sampler the files do not declare reads a texture the
// engine provides, such as the stand in for its bindless textures.
type Texture struct {
	Name    string
	Type    string
	Sampler *Sampler
}

// Uniform is one value of the constant buffers: a scalar, a vector or a
// matrix, as OpenGL names it, such as cb_PdxCamera.CameraPosition.
type Uniform struct {
	Name       string
	Kind       Kind
	VectorSize int
	Columns    int
}

// Kind is the type of the values of a uniform.
type Kind int

const (
	KindOther Kind = iota
	KindFloat
	KindInt
	KindUint
	KindBool
)

// Attribute is an input of the vertex stage.
type Attribute struct {
	Variable

	// Location is the vertex stream it reads; one past the streams of a mesh
	// reads nothing, and so the value OpenGL gives a stream not bound.
	Location int
}

// Variable is a member of a struct or a constant buffer.
type Variable struct {
	Type string
	Name string

	// Array is the length of an array, as written, or empty.
	Array string

	// Semantic is the semantic of a member of a vertex struct.
	Semantic string
}

// The vertex streams of a mesh, by the location raylib binds each to, and the
// members of the input of the engine's mesh shaders that read them.
var streamLocations = map[string]int{
	"Position": 0,
	"UV0":      1,
	"Normal":   2,
	"Color":    3,
	"Tangent":  4,
	"UV1":      5,
}

// firstFreeLocation is the first location no stream of a mesh is bound to.
const firstFreeLocation = 8

// file reads a file, once.
func (l *Library) file(name string) (*File, error) {
	if file, ok := l.files[name]; ok {
		return file, nil
	}

	data, err := l.source.ReadFile(name)
	if err != nil {
		return nil, err
	}

	file, err := ParseFile(name, string(data))
	if err != nil {
		return nil, err
	}

	l.files[name] = file

	return file, nil
}

// closure returns a file and every file it includes, each once, includes
// before the files that include them, the way the engine pastes them
// together.
func (l *Library) closure(name string) ([]*File, error) {
	var (
		ordered []*File
		visited = map[string]bool{}
		visit   func(name, from string) error
	)

	visit = func(name, from string) error {
		key := strings.ToLower(name)
		if visited[key] {
			return nil
		}

		visited[key] = true

		file, err := l.file(name)
		if err != nil {
			if from != "" {
				return fmt.Errorf("%s includes %w", from, err)
			}

			return err
		}

		for _, include := range file.Includes {
			if err := visit(path.Join(folderFX, include), name); err != nil {
				return err
			}
		}

		ordered = append(ordered, file)

		return nil
	}

	if err := visit(name, ""); err != nil {
		return nil, err
	}

	return ordered, nil
}

// StageError is a stage that did not compile: which stage, the step of the
// compiler that failed, and what the compiler said.
type StageError struct {
	Effect string
	Stage  string
	Step   string
	Log    string
}

func (e *StageError) Error() string {
	return fmt.Sprintf("effect %s, %s stage: %s: %s", e.Effect, e.Stage, e.Step, firstLine(e.Log))
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return line
}

// Program builds an effect of a shader file, such as standard of
// gfx/FX/pdxmesh.shader, with further defines, such as those a mesh's
// settings add, and compiles it to GLSL.
func (l *Library) Program(shaderFile, effectName string, defines []string) (*Program, error) {
	program, err := l.Assemble(shaderFile, effectName, defines)
	if err != nil {
		return nil, err
	}

	for _, stage := range []struct {
		name   string
		kind   glslcross.Stage
		source string
		glsl   *string
	}{
		{"vertex", glslcross.Vertex, program.VertexHLSL, &program.Vertex},
		{"pixel", glslcross.Pixel, program.PixelHLSL, &program.Pixel},
	} {
		compiled, err := glslcross.Compile(stage.source, stage.kind, entryPoint)
		if err != nil {
			stageErr := &StageError{Effect: effectName, Stage: stage.name, Log: err.Error()}

			if compileErr, ok := err.(*glslcross.Error); ok {
				stageErr.Step, stageErr.Log = compileErr.Step, compileErr.Log
			}

			return program, stageErr
		}

		*stage.glsl = compiled.GLSL

		program.addTextures(compiled.GLSL, compiled.Samplers)
		program.addUniforms(compiled.Uniforms)
	}

	return program, nil
}

// glslSampler finds the samplers a GLSL stage declares.
var glslSampler = regexp.MustCompile(`(?m)^uniform\s+((?:[iu])?sampler\w*)\s+(\w+)\s*;`)

// addTextures lists the samplers of a stage, and ties each to the sampler of
// the shader files it stands for. glslang names the texture of a sampler
// after the sampler and the member that holds it: DiffuseMap._Texture.
func (p *Program) addTextures(glsl string, samplers []glslcross.Sampler) {
	for _, match := range glslSampler.FindAllStringSubmatch(glsl, -1) {
		kind, name := match[1], match[2]

		if slices.ContainsFunc(p.Textures, func(known Texture) bool { return known.Name == name }) {
			continue
		}

		texture := Texture{Name: name, Type: kind}

		for _, sampler := range samplers {
			if sampler.Name != name {
				continue
			}

			owner, _, _ := strings.Cut(sampler.Image, ".")

			for _, declared := range p.declared {
				if declared.Name == owner {
					texture.Sampler = declared

					break
				}
			}
		}

		p.Textures = append(p.Textures, texture)
	}
}

func (p *Program) addUniforms(uniforms []glslcross.Uniform) {
	for _, uniform := range uniforms {
		if slices.ContainsFunc(p.Uniforms, func(known Uniform) bool { return known.Name == uniform.Name }) {
			continue
		}

		p.Uniforms = append(p.Uniforms, Uniform{
			Name:       uniform.Name,
			Kind:       Kind(uniform.Kind),
			VectorSize: uniform.VectorSize,
			Columns:    uniform.Columns,
		})
	}
}

// Assemble builds the HLSL of both stages of an effect, without compiling
// it.
func (l *Library) Assemble(shaderFile, effectName string, defines []string) (*Program, error) {
	files, err := l.closure(shaderFile)
	if err != nil {
		return nil, err
	}

	main := files[len(files)-1]

	effect, ok := main.Effects[effectName]
	if !ok {
		return nil, fmt.Errorf("%s has no effect %s", shaderFile, effectName)
	}

	var preludeCode []string

	for _, prelude := range preludes {
		data, err := l.source.ReadFile(path.Join(folderFX, prelude))
		if err != nil {
			return nil, fmt.Errorf("the prelude %s: %w", prelude, err)
		}

		preludeCode = append(preludeCode, string(data))
	}

	builder := &builder{files: files, defines: append(slices.Clone(defines), effect.Defines...), prelude: preludeCode}
	program := &Program{Effect: effect, States: map[string]*State{}, declared: builder.samplers()}

	// An effect that names no state of a kind has the one of its file named
	// after the kind, if there is one: tree.shader declares BlendState
	// BlendState, with alpha to coverage, which no effect of it names.
	for kind, name := range map[string]string{
		"BlendState":        effect.BlendState,
		"DepthStencilState": effect.DepthStencilState,
		"RasterizerState":   effect.RasterizerState,
	} {
		if state := builder.state(name); state != nil {
			program.States[kind] = state
		} else if strings.Trim(name, `"`) == "" {
			if state, ok := files[len(files)-1].States[kind]; ok {
				program.States[kind] = state
			}
		}
	}

	vertexMain, ok := builder.main(effect.VertexShader, StageVertex)
	if !ok {
		return nil, fmt.Errorf("effect %s: there is no vertex shader %s", effectName, effect.VertexShader)
	}

	pixelMain, ok := builder.main(effect.PixelShader, StagePixel)
	if !ok {
		return nil, fmt.Errorf("effect %s: there is no pixel shader %s", effectName, effect.PixelShader)
	}

	if program.VertexHLSL, err = builder.stage(StageVertex, vertexMain, program); err != nil {
		return nil, fmt.Errorf("effect %s, vertex shader %s: %w", effectName, vertexMain.Name, err)
	}

	if program.PixelHLSL, err = builder.stage(StagePixel, pixelMain, program); err != nil {
		return nil, fmt.Errorf("effect %s, pixel shader %s: %w", effectName, pixelMain.Name, err)
	}

	return program, nil
}

// builder assembles the stages of one program.
type builder struct {
	files   []*File
	defines []string
	prelude []string
}

func (b *builder) state(name string) *State {
	name = strings.Trim(name, `"`)
	if name == "" {
		return nil
	}

	for index := len(b.files) - 1; index >= 0; index-- {
		if state, ok := b.files[index].States[name]; ok {
			return state
		}
	}

	return nil
}

// main finds an entry point of a stage by name.
func (b *builder) main(name string, stage Stage) (*Main, bool) {
	for index := len(b.files) - 1; index >= 0; index-- {
		for _, item := range b.files[index].Items {
			if item.Main != nil && item.Main.Name == name && (item.Stage == stage || item.Stage == StageBoth) {
				return item.Main, true
			}
		}
	}

	return nil, false
}

// samplers lists the samplers the files declare.
func (b *builder) samplers() []*Sampler {
	var samplers []*Sampler

	for _, file := range b.files {
		for _, item := range file.Items {
			switch {
			case item.Sampler != nil:
				samplers = append(samplers, item.Sampler)
			case item.Main != nil:
				samplers = append(samplers, item.Main.Samplers...)
			}
		}
	}

	return samplers
}

// stage assembles the HLSL of one stage.
func (b *builder) stage(stage Stage, main *Main, program *Program) (string, error) {
	var out strings.Builder

	out.WriteString("#define PDX_HLSL\n")

	for _, define := range engineDefines {
		fmt.Fprintf(&out, "#define %s\n", define)
	}

	if stage == StageVertex {
		out.WriteString("#define VERTEX_SHADER\n")
	} else {
		out.WriteString("#define PIXEL_SHADER\n")
	}

	for _, define := range b.defines {
		name, value := splitDefine(define)

		// The mesh settings of the shipped files hold an empty define now
		// and then, which defines nothing.
		if name != "" {
			fmt.Fprintf(&out, "#define %s %s\n", name, value)
		}
	}

	for _, prelude := range b.prelude {
		out.WriteString(prelude)
		out.WriteString("\n")
	}

	out.WriteString(engineCode)

	members := map[string][]member{}

	// The vertex structs, with the semantics that tie them to the stages.
	// The members of the input of the vertex stage carry the location of
	// the vertex stream each reads.
	for _, file := range b.files {
		for _, item := range file.Items {
			if item.Struct == nil || members[item.Struct.Name] != nil {
				continue
			}

			parsed := parseMembers(item.Struct.Raw, b.defines)
			members[item.Struct.Name] = parsed

			vertexInput := stage == StageVertex && item.Struct.Name == main.Input

			fmt.Fprintf(&out, "struct %s\n{\n", item.Struct.Name)

			next := firstFreeLocation

			for _, entry := range parsed {
				if entry.directive != "" {
					out.WriteString(entry.directive + "\n")

					continue
				}

				out.WriteString("\t")

				if vertexInput && !isSystemValue(entry.Semantic) {
					location, ok := streamLocations[entry.Name]
					if !ok {
						location = next
						next++
					}

					fmt.Fprintf(&out, "[[vk::location(%d)]] ", location)
					program.Attributes = append(program.Attributes, Attribute{Variable: entry.Variable, Location: location})
				}

				fmt.Fprintf(&out, "%s %s%s", entry.Type, entry.Name, arraySuffix(entry.Array))

				if entry.Semantic != "" {
					fmt.Fprintf(&out, " : %s", entry.Semantic)
				}

				out.WriteString(";\n")
			}

			out.WriteString("};\n")
		}
	}

	// The constant buffers, the samplers and the buffers the stage reads. A
	// constant buffer with a member of a struct type waits for the code
	// that defines the struct: the code of an include uses samplers and
	// buffers the files including it declare, so the declarations come
	// first, but a struct is only known where its code defines it.
	type waitingBuffer struct {
		types []string
		text  string
	}

	var waiting []waitingBuffer

	seenBuffers := map[string]bool{}

	for _, file := range b.files {
		for _, item := range file.Items {
			if !item.visibleIn(stage) {
				continue
			}

			switch {
			case item.Buffer != nil && !seenBuffers[item.Buffer.Name]:
				seenBuffers[item.Buffer.Name] = true

				// The vertex structs are written already, and may be the
				// types of members, as LightDirT is in portrait.shader.
				text, types := b.constantBuffer(item.Buffer)
				types = slices.DeleteFunc(types, func(kind string) bool { return members[kind] != nil })

				if len(types) == 0 {
					out.WriteString(text)
				} else {
					waiting = append(waiting, waitingBuffer{types: types, text: text})
				}

			case item.Sampler != nil:
				fmt.Fprintf(&out, "%s %s;\n", samplerType(item.Sampler), item.Sampler.Name)

			case item.Declaration != "":
				out.WriteString(item.Declaration)

			case item.BufferTexture != nil:
				kind, ok := bufferType(item.BufferTexture.Type)
				if !ok {
					// A buffer of structures, read by compute shaders.
					continue
				}

				fmt.Fprintf(&out, "%s %s;\n", kind, item.BufferTexture.Name)

				if !slices.Contains(program.BufferTextures, item.BufferTexture) {
					program.BufferTextures = append(program.BufferTextures, item.BufferTexture)
				}
			}
		}
	}

	// The samplers and the constant buffers the entry point declares for
	// itself.
	for _, sampler := range main.Samplers {
		fmt.Fprintf(&out, "%s %s;\n", samplerType(sampler), sampler.Name)
	}

	for _, buffer := range main.Buffers {
		text, _ := b.constantBuffer(buffer)
		out.WriteString(text)
	}

	// PDX_IsFrontFace says whether the pixel stage draws the front of a
	// triangle. The GLSL prelude maps it to gl_FrontFacing; for HLSL the
	// engine passes it to the entry point, which hands it on through this.
	if stage == StagePixel {
		out.WriteString("static bool PDX_IsFrontFace = true;\n")
	}

	// The code shared by both stages first, then the code of the stage: the
	// shared code of a file included late is what the code of a stage of a
	// file included early relies on, as pdxmesh.fxh relies on GetHeight of
	// heightmap.fxh.
	defined := map[string]bool{}

	for _, shared := range []bool{true, false} {
		for _, file := range b.files {
			for _, item := range file.Items {
				if item.Code == "" || (item.Stage == StageBoth) != shared || !item.visibleIn(stage) {
					continue
				}

				fmt.Fprintf(&out, "// %s\n%s\n", file.Path, instantiateTemplates(item.Code))

				for _, buffer := range waiting {
					for _, kind := range buffer.types {
						if definesStruct(item.Code, kind) {
							defined[kind] = true
						}
					}
				}

				waiting = slices.DeleteFunc(waiting, func(buffer waitingBuffer) bool {
					for _, kind := range buffer.types {
						if !defined[kind] {
							return false
						}
					}

					out.WriteString(buffer.text)

					return true
				})
			}
		}
	}

	// Buffers of a type no code defines, which the compiler reports.
	for _, buffer := range waiting {
		out.WriteString(buffer.text)
	}

	if _, ok := members[main.Input]; !ok {
		return "", fmt.Errorf("there is no vertex struct %s for the input", main.Input)
	}

	// A stage returns a struct of the file's, a colour, or, as the pixel
	// stages of the effects a shadow is cast with do, which draw depth
	// alone, nothing.
	output := main.Output
	if _, ok := members[output]; !ok {
		switch output {
		case "PDX_COLOR":
			output = "float4"
		case "void":
		default:
			return "", fmt.Errorf("there is no vertex struct %s for the output", output)
		}
	}

	fmt.Fprintf(&out, "#define PDX_MAIN %s PdxMain( %s Input )\n%s\n#undef PDX_MAIN\n", output, main.Input, main.Code)

	parameters, setup := fmt.Sprintf("%s Input", main.Input), ""
	if stage == StagePixel {
		parameters += ", bool FrontFace : SV_IsFrontFace"
		setup = "\tPDX_IsFrontFace = FrontFace;\n"
	}

	switch output {
	case "void":
		fmt.Fprintf(&out, "void %s( %s )\n{\n%s\tPdxMain( Input );\n}\n", entryPoint, parameters, setup)
	case "float4":
		fmt.Fprintf(&out, "float4 %s( %s ) : PDX_COLOR\n{\n%s\treturn PdxMain( Input );\n}\n", entryPoint, parameters, setup)
	default:
		fmt.Fprintf(&out, "%s %s( %s )\n{\n%s\treturn PdxMain( Input );\n}\n", output, entryPoint, parameters, setup)
	}

	return out.String(), nil
}

// constantBuffer writes a constant buffer, and returns the struct types its
// members are of, which code has to define first.
func (b *builder) constantBuffer(buffer *Buffer) (string, []string) {
	var (
		out   strings.Builder
		types []string
	)

	name := buffer.Name
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "PdxConstantBuffer" + name
	}

	fmt.Fprintf(&out, "cbuffer %s\n{\n", name)

	for _, entry := range parseMembers(buffer.Raw, b.defines) {
		if entry.directive != "" {
			out.WriteString(entry.directive + "\n")

			continue
		}

		array := entry.Array
		if length, err := strconv.Atoi(array); err == nil && length < minimumBufferArray {
			array = strconv.Itoa(minimumBufferArray)
		}

		fmt.Fprintf(&out, "\t%s %s%s;\n", entry.Type, entry.Name, arraySuffix(array))

		if !isBuiltinType(entry.Type) && !slices.Contains(types, entry.Type) {
			types = append(types, entry.Type)
		}
	}

	out.WriteString("};\n")

	return out.String(), types
}

func (i Item) visibleIn(stage Stage) bool {
	return i.Stage == StageBoth || i.Stage == stage
}

// isSystemValue reports whether a semantic is a value the pipeline itself
// provides rather than a vertex stream.
func isSystemValue(semantic string) bool {
	return semantic == "PDX_VertexID" || semantic == "PDX_InstanceID" || strings.HasPrefix(semantic, "SV_")
}

func arraySuffix(array string) string {
	if array == "" {
		return ""
	}

	return "[" + array + "]"
}

// samplerType is the type a sampler is declared with, in the names the
// prelude maps to the textures and samplers of HLSL.
func samplerType(sampler *Sampler) string {
	switch {
	case sampler.Compare:
		return "PdxTextureSampler2DCmp"
	case strings.EqualFold(sampler.Type, "Cube"):
		return "PdxTextureSamplerCube"
	case strings.EqualFold(sampler.Type, "2darray"):
		return "PdxTextureSampler2DArray"
	case strings.EqualFold(sampler.Type, "3D"):
		return "PdxTextureSampler3D"
	}

	return "PdxTextureSampler2D"
}

// bufferType is the type a buffer texture is declared with.
func bufferType(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "float":
		return "PdxBufferFloat", true
	case "float2":
		return "PdxBufferFloat2", true
	case "float3":
		return "PdxBufferFloat3", true
	case "float4":
		return "PdxBufferFloat4", true
	case "int":
		return "PdxBufferInt", true
	case "int2":
		return "PdxBufferInt2", true
	case "int4":
		return "PdxBufferInt4", true
	case "uint":
		return "PdxBufferUint", true
	case "uint2":
		return "PdxBufferUint2", true
	case "uint4":
		return "PdxBufferUint4", true
	}

	return "", false
}

// templateTypes are the types a template function is written out for.
var templateTypes = []string{
	"float", "float2", "float3", "float4",
	"int", "int2", "int3", "int4",
	"uint", "uint2", "uint3", "uint4",
}

var templateHeader = regexp.MustCompile(`template\s*<\s*typename\s+(\w+)\s*>`)

// instantiateTemplates writes every template function of code out once for
// each of the templateTypes, as overloads.
//
// HLSL 2021 has templates, and Europa Universalis 5 writes a function as one:
// LerpBilinear, in terrain2_materials.fxh. glslang reads the HLSL of before,
// which has none. A template function compiles as if it were written out for
// each type it is called with, so writing it out for every type it can be
// called with means the same, with the overloads no call picks left unused.
func instantiateTemplates(code string) string {
	for {
		match := templateHeader.FindStringSubmatchIndex(code)
		if match == nil {
			return code
		}

		parameter := code[match[2]:match[3]]

		// The function runs from after the header to the brace that closes
		// its body.
		open := strings.IndexByte(code[match[1]:], '{')
		if open < 0 {
			return code
		}

		end, depth := -1, 0

		for index := match[1] + open; index < len(code); index++ {
			switch code[index] {
			case '{':
				depth++
			case '}':
				depth--
			}

			if depth == 0 {
				end = index + 1

				break
			}
		}

		if end < 0 {
			return code
		}

		function := code[match[1]:end]
		word := regexp.MustCompile(`\b` + regexp.QuoteMeta(parameter) + `\b`)

		var instances strings.Builder

		for _, kind := range templateTypes {
			instances.WriteString(word.ReplaceAllString(function, kind))
			instances.WriteString("\n")
		}

		code = code[:match[0]] + instances.String() + code[end:]
	}
}

// splitDefine splits a define as the files write it into its name and its
// value: NAME, NAME=VALUE or NAME VALUE, the value being any text, such as
// 0.76702f * 2.0f.
func splitDefine(define string) (name, value string) {
	define = strings.TrimSpace(define)

	end := strings.IndexFunc(define, func(r rune) bool { return r == '=' || r == ' ' || r == '\t' })
	if end < 0 {
		return define, ""
	}

	return define[:end], strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(define[end:]), "="))
}

// isBuiltinType reports whether a type is one the language has, as written
// in the shader files, rather than a struct of theirs.
func isBuiltinType(kind string) bool {
	for _, base := range []string{"float", "half", "int", "uint", "bool", "double", "min16", "matrix", "vector"} {
		if strings.HasPrefix(kind, base) {
			return true
		}
	}

	return kind == "Quaternion"
}

// definesStruct reports whether code defines a struct of a name.
func definesStruct(code, name string) bool {
	for index := strings.Index(code, name); index >= 0; {
		before := strings.TrimRight(code[:index], " \t\r\n")
		after := index + len(name)

		if strings.HasSuffix(before, "struct") && (after == len(code) || !isWordByte(code[after])) {
			return true
		}

		next := strings.Index(code[after:], name)
		if next < 0 {
			return false
		}

		index = after + next
	}

	return false
}

// member is a line of the body of a vertex struct or a constant buffer: a
// declaration, or a preprocessor directive kept for the compiler.
type member struct {
	Variable

	directive string
}

var declaration = regexp.MustCompile(`^(\w+)\s+(\w+)\s*(?:\[\s*([^\]]*?)\s*\])?\s*(?::\s*(\w+))?\s*;?$`)

// parseMembers reads the body of a vertex struct or a constant buffer. The
// lines starting with @ are switched by the defines here, the way the engine
// does before it compiles; # directives are kept for the compiler, and any
// other # starts a comment.
func parseMembers(raw string, defines []string) []member {
	var (
		members []member

		// active holds, for each @if level, whether its lines are kept.
		active []bool
	)

	defined := func(name string) bool {
		return slices.ContainsFunc(defines, func(define string) bool {
			defined, _ := splitDefine(define)

			return defined == name
		})
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if comment := strings.Index(line, "//"); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "@") {
			fields := strings.Fields(line)

			switch fields[0] {
			case "@ifdef":
				active = append(active, len(fields) > 1 && defined(fields[1]))
			case "@ifndef":
				active = append(active, len(fields) < 2 || !defined(fields[1]))
			case "@else":
				if len(active) > 0 {
					active[len(active)-1] = !active[len(active)-1]
				}
			case "@endif":
				if len(active) > 0 {
					active = active[:len(active)-1]
				}
			}

			continue
		}

		if slices.Contains(active, false) {
			continue
		}

		if strings.HasPrefix(line, "#") {
			if isDirective(line) {
				members = append(members, member{directive: line})
			}

			continue
		}

		if comment := strings.Index(line, "#"); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}

		match := declaration.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		members = append(members, member{Variable: Variable{Type: match[1], Name: match[2], Array: match[3], Semantic: match[4]}})
	}

	return members
}

func isDirective(line string) bool {
	word := strings.Fields(strings.TrimPrefix(line, "#"))
	if len(word) == 0 {
		return false
	}

	switch word[0] {
	case "if", "ifdef", "ifndef", "else", "elif", "endif", "define", "undef":
		return true
	}

	return false
}
