//go:build uitest && windows

package shader

import (
	"fmt"
	"syscall"
	"unsafe"
)

// glCompiler compiles shaders through OpenGL directly rather than through
// raylib, for the whole of what the driver reports. raylib-go hands raylib's
// log to Go through a buffer of 256 bytes, which the log of a long shader
// overflows.
//
// The functions are looked up with wglGetProcAddress, which needs the OpenGL
// context current on the calling thread.
type glCompiler struct {
	createShader, shaderSource, compileShader, getShaderiv, getShaderInfoLog, deleteShader   uintptr
	createProgram, attachShader, linkProgram, getProgramiv, getProgramInfoLog, deleteProgram uintptr
}

const (
	glFragmentShader = 0x8B30
	glVertexShader   = 0x8B31
	glCompileStatus  = 0x8B81
	glLinkStatus     = 0x8B82
	glInfoLogLength  = 0x8B84
)

func newGLCompiler() (*glCompiler, error) {
	getProc := syscall.NewLazyDLL("opengl32.dll").NewProc("wglGetProcAddress")

	compiler := &glCompiler{}

	for name, slot := range map[string]*uintptr{
		"glCreateShader":      &compiler.createShader,
		"glShaderSource":      &compiler.shaderSource,
		"glCompileShader":     &compiler.compileShader,
		"glGetShaderiv":       &compiler.getShaderiv,
		"glGetShaderInfoLog":  &compiler.getShaderInfoLog,
		"glDeleteShader":      &compiler.deleteShader,
		"glCreateProgram":     &compiler.createProgram,
		"glAttachShader":      &compiler.attachShader,
		"glLinkProgram":       &compiler.linkProgram,
		"glGetProgramiv":      &compiler.getProgramiv,
		"glGetProgramInfoLog": &compiler.getProgramInfoLog,
		"glDeleteProgram":     &compiler.deleteProgram,
	} {
		cName, err := syscall.BytePtrFromString(name)
		if err != nil {
			return nil, err
		}

		address, _, _ := getProc.Call(uintptr(unsafe.Pointer(cName)))
		if address == 0 {
			return nil, fmt.Errorf("OpenGL has no %s", name)
		}

		*slot = address
	}

	return compiler, nil
}

// compile compiles both stages and links them, and returns what failed and
// the driver's log, or empty strings when all went well.
func (c *glCompiler) compile(vertex, pixel string) (failed, log string) {
	vertexShader, log := c.stage(glVertexShader, vertex)
	if log != "" {
		return "vertex", log
	}
	defer syscall.SyscallN(c.deleteShader, vertexShader)

	pixelShader, log := c.stage(glFragmentShader, pixel)
	if log != "" {
		return "pixel", log
	}
	defer syscall.SyscallN(c.deleteShader, pixelShader)

	program, _, _ := syscall.SyscallN(c.createProgram)
	defer syscall.SyscallN(c.deleteProgram, program)

	syscall.SyscallN(c.attachShader, program, vertexShader)
	syscall.SyscallN(c.attachShader, program, pixelShader)
	syscall.SyscallN(c.linkProgram, program)

	var status int32
	syscall.SyscallN(c.getProgramiv, program, glLinkStatus, uintptr(unsafe.Pointer(&status)))

	if status == 0 {
		return "link", c.infoLog(c.getProgramiv, c.getProgramInfoLog, program)
	}

	return "", ""
}

// stage compiles one stage, and returns its log when it does not compile.
func (c *glCompiler) stage(kind uintptr, source string) (uintptr, string) {
	shader, _, _ := syscall.SyscallN(c.createShader, kind)

	text := append([]byte(source), 0)
	pointer := &text[0]
	syscall.SyscallN(c.shaderSource, shader, 1, uintptr(unsafe.Pointer(&pointer)), 0)
	syscall.SyscallN(c.compileShader, shader)

	var status int32
	syscall.SyscallN(c.getShaderiv, shader, glCompileStatus, uintptr(unsafe.Pointer(&status)))

	if status != 0 {
		return shader, ""
	}

	log := c.infoLog(c.getShaderiv, c.getShaderInfoLog, shader)
	syscall.SyscallN(c.deleteShader, shader)

	if log == "" {
		log = "the driver gave no reason"
	}

	return 0, log
}

func (c *glCompiler) infoLog(get, read, object uintptr) string {
	var length int32
	syscall.SyscallN(get, object, glInfoLogLength, uintptr(unsafe.Pointer(&length)))

	if length <= 1 {
		return ""
	}

	buffer := make([]byte, length)
	syscall.SyscallN(read, object, uintptr(length), 0, uintptr(unsafe.Pointer(&buffer[0])))

	return string(buffer[:length-1])
}
