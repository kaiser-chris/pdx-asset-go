// Package glslcross compiles HLSL to GLSL, with libraries of the Khronos
// Group: glslang compiles the HLSL to SPIR-V, SPIRV-Tools legalizes it, and
// SPIRV-Cross writes the SPIR-V out as GLSL.
//
// Legalizing is what makes the games' HLSL work at all: they hold a texture
// and its sampler together in a struct and hand it to functions as one, which
// SPIR-V cannot express until the functions are inlined and the structs taken
// apart. glslang does that by running SPIRV-Tools' optimizer over what it
// wrote.
//
// The games write their shaders for Direct3D and Vulkan in HLSL; raylib draws
// with OpenGL. The sources of the libraries are vendored below this folder,
// at the release of vulkan-sdk-1.4.363.0, and compiled by cgo along with the
// package: only what compiling HLSL to SPIR-V, legalizing it and writing GLSL
// needs, without tests, tools or other backends. The tables SPIRV-Tools
// generates at build time from the grammar of SPIRV-Headers are vendored as
// generated, in spirvtools/generated. Their licences are next to them.
//
// glslang has declared its HLSL front end deprecated, to be removed at its next
// major version, so a later release cannot be taken over without checking that
// it still reads HLSL.
package glslcross

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo CXXFLAGS: -std=c++17 -w -I${SRCDIR} -I${SRCDIR}/glslang -I${SRCDIR}/spirvcross
#cgo CXXFLAGS: -I${SRCDIR}/spirvtools -I${SRCDIR}/spirvtools/include -I${SRCDIR}/spirvtools/generated -I${SRCDIR}/spirvheaders/include
#cgo CXXFLAGS: -DENABLE_HLSL -DENABLE_SPIRV -DENABLE_OPT=1
#cgo CXXFLAGS: -DSPIRV_CROSS_C_API_GLSL=1 -DSPIRV_CROSS_C_API_HLSL=0 -DSPIRV_CROSS_C_API_MSL=0 -DSPIRV_CROSS_C_API_CPP=0 -DSPIRV_CROSS_C_API_REFLECT=0
#cgo windows CXXFLAGS: -DGLSLANG_OSINCLUDE_WIN32
#cgo !windows CXXFLAGS: -DGLSLANG_OSINCLUDE_UNIX
#cgo !windows LDFLAGS: -lpthread

// On Windows the C++ runtime of MinGW, libstdc++ with libgcc and
// winpthreads, comes as DLLs that Windows does not have: a program that loads
// them starts only where a MinGW of the same build is on PATH, and fails with
// a missing entry point where another is. Linking them in keeps every
// program built with this package, tests included, starting anywhere.
#cgo windows LDFLAGS: -static

#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// Stage is the stage of the pipeline a shader is compiled for.
type Stage int

const (
	Vertex Stage = iota
	Pixel
)

// GLSLVersion is the version of GLSL written: 3.30, the version of the OpenGL
// raylib draws with by default.
const GLSLVersion = 330

// Sampler is a sampler of the GLSL, which combines a texture and a sampler of
// the HLSL. Image and Sampler are their names in the HLSL as glslang passes
// them on: a member of a struct is named after the struct and the member, as
// in DiffuseMap._Texture.
type Sampler struct {
	Name    string
	Image   string
	Sampler string
}

// Kind is the type of the values of a uniform.
type Kind int

const (
	Other Kind = iota
	Float
	Int
	Uint
	Bool
)

// Uniform is one value of the constant buffers of a stage, which become
// plain uniforms named cb_ and the buffer's name, such as
// cb_PdxCamera.CameraPosition. An array is listed element by element, and a
// struct member by member.
type Uniform struct {
	Name string
	Kind Kind

	// VectorSize is 1 for a scalar, and Columns more than 1 for a matrix.
	VectorSize int
	Columns    int
}

// Result is a stage compiled to GLSL.
type Result struct {
	GLSL     string
	Samplers []Sampler
	Uniforms []Uniform

	// Warnings are what glslang had to say about the SPIR-V it wrote.
	Warnings string
}

// Error is a stage that did not compile: the step that failed, such as parse
// for the HLSL or cross for the GLSL, and what the library logged.
type Error struct {
	Step string
	Log  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Step, e.Log)
}

var (
	initialize sync.Once
	initErr    error

	// glslang keeps state for the process; compiling one stage at a time
	// keeps it out of the question whether that state may be shared.
	compiling sync.Mutex
)

// Compile compiles one stage of HLSL to GLSL. Entry is the function the
// stage starts at.
func Compile(source string, stage Stage, entry string) (*Result, error) {
	initialize.Do(func() {
		if C.glslcross_initialize() == 0 {
			initErr = errors.New("glslang could not be initialized")
		}
	})

	if initErr != nil {
		return nil, initErr
	}

	compiling.Lock()
	defer compiling.Unlock()

	cSource := C.CString(source)
	defer C.free(unsafe.Pointer(cSource))

	cEntry := C.CString(entry)
	defer C.free(unsafe.Pointer(cEntry))

	pixel := C.int(0)
	if stage == Pixel {
		pixel = 1
	}

	var result C.glslcross_result

	C.glslcross_compile(cSource, pixel, cEntry, C.int(GLSLVersion), &result)
	defer C.glslcross_free(&result)

	if result.glsl == nil {
		return nil, &Error{Step: C.GoString(result.failed_step), Log: C.GoString(result.log)}
	}

	compiled := &Result{GLSL: C.GoString(result.glsl), Warnings: C.GoString(result.warnings)}

	if result.sampler_count > 0 {
		for _, sampler := range unsafe.Slice(result.samplers, result.sampler_count) {
			compiled.Samplers = append(compiled.Samplers, Sampler{
				Name:    C.GoString(sampler.name),
				Image:   C.GoString(sampler.image),
				Sampler: C.GoString(sampler.sampler),
			})
		}
	}

	if result.uniform_count > 0 {
		for _, uniform := range unsafe.Slice(result.uniforms, result.uniform_count) {
			compiled.Uniforms = append(compiled.Uniforms, Uniform{
				Name:       C.GoString(uniform.name),
				Kind:       Kind(uniform.kind),
				VectorSize: int(uniform.vector_size),
				Columns:    int(uniform.columns),
			})
		}
	}

	return compiled, nil
}
