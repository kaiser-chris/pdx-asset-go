package render

/*
#cgo windows LDFLAGS: -lopengl32
#cgo linux LDFLAGS: -lGL
#cgo darwin LDFLAGS: -framework OpenGL

#ifdef __APPLE__
#include <OpenGL/gl.h>
#else
#include <GL/gl.h>
#endif

// What OpenGL 1.4 added, which the headers of OpenGL 1.1 that Windows ships
// do not define.
#define PDX_DEPTH_COMPONENT24 0x81A6
#define PDX_TEXTURE_COMPARE_MODE 0x884C
#define PDX_TEXTURE_COMPARE_FUNC 0x884D
#define PDX_COMPARE_REF_TO_TEXTURE 0x884E
#define PDX_TEXTURE_CUBE_MAP 0x8513
#define PDX_TEXTURE_MAX_LEVEL 0x813D
#define PDX_CLAMP_TO_EDGE 0x812F
#define PDX_POLYGON_OFFSET_FILL 0x8037

// pdx_lit_shadow_map makes a depth texture of one pixel that every
// comparison passes.
static unsigned int pdx_lit_shadow_map(void) {
	GLuint id = 0;
	GLfloat depth = 1.0f;

	glGenTextures(1, &id);
	glBindTexture(GL_TEXTURE_2D, id);
	glTexImage2D(GL_TEXTURE_2D, 0, PDX_DEPTH_COMPONENT24, 1, 1, 0, GL_DEPTH_COMPONENT, GL_FLOAT, &depth);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP);
	glTexParameteri(GL_TEXTURE_2D, PDX_TEXTURE_COMPARE_MODE, PDX_COMPARE_REF_TO_TEXTURE);
	glTexParameteri(GL_TEXTURE_2D, PDX_TEXTURE_COMPARE_FUNC, GL_ALWAYS);
	glBindTexture(GL_TEXTURE_2D, 0);

	return id;
}

// pdx_shadow_depth makes a depth texture of 24 bits a shadow map is drawn
// into and compared with: a point is lit where its depth is less than or
// equal to the map's, filtered between the four pixels around it.
static unsigned int pdx_shadow_depth(int size) {
	GLuint id = 0;

	glGenTextures(1, &id);
	glBindTexture(GL_TEXTURE_2D, id);
	glTexImage2D(GL_TEXTURE_2D, 0, PDX_DEPTH_COMPONENT24, size, size, 0, GL_DEPTH_COMPONENT, GL_UNSIGNED_INT, 0);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, PDX_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, PDX_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, PDX_TEXTURE_COMPARE_MODE, PDX_COMPARE_REF_TO_TEXTURE);
	glTexParameteri(GL_TEXTURE_2D, PDX_TEXTURE_COMPARE_FUNC, GL_LEQUAL);
	glBindTexture(GL_TEXTURE_2D, 0);

	return id;
}

// pdx_no_color has the bound framebuffer draw and read no colour, which one
// of nothing but depth needs to be complete.
static void pdx_no_color(void) {
	glDrawBuffer(GL_NONE);
	glReadBuffer(GL_NONE);
}

// pdx_depth_bias offsets the depth of what is drawn, as Direct3D's
// rasterizer states do: by a slope and by steps of the depth buffer.
static void pdx_depth_bias(float slope, float steps) {
	if (slope == 0.0f && steps == 0.0f) {
		glDisable(PDX_POLYGON_OFFSET_FILL);
		return;
	}

	glEnable(PDX_POLYGON_OFFSET_FILL);
	glPolygonOffset(slope, steps);
}

// pdx_cube_levels says how many levels of a cube map there are.
static void pdx_cube_levels(unsigned int id, int levels) {
	glBindTexture(PDX_TEXTURE_CUBE_MAP, id);
	glTexParameteri(PDX_TEXTURE_CUBE_MAP, PDX_TEXTURE_MAX_LEVEL, levels - 1);
	glBindTexture(PDX_TEXTURE_CUBE_MAP, 0);
}
*/
import "C"

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// litShadowMap is what a shadow sampler reads: nothing casts a shadow.
//
// The games read their shadow map through a comparison sampler, which in
// OpenGL is a sampler2DShadow and has to read a depth texture set to compare,
// or what it returns is undefined. raylib makes no such texture, so it is
// made here, with the calls of OpenGL 1.1 the platforms export. The
// comparison always passes, so that every shadow lookup is fully lit.
func litShadowMap() rl.Texture2D {
	return rl.Texture2D{ID: uint32(C.pdx_lit_shadow_map()), Width: 1, Height: 1, Mipmaps: 1}
}

// setCubeLevels tells OpenGL how many levels a cube map has. One with fewer
// than its size calls for is incomplete otherwise, and reads black.
func setCubeLevels(cube rl.Texture2D, levels int) {
	C.pdx_cube_levels(C.uint(cube.ID), C.int(levels))
}

// shadowDepth makes the depth texture of a shadow map of a size.
func shadowDepth(size int32) rl.Texture2D {
	return rl.Texture2D{ID: uint32(C.pdx_shadow_depth(C.int(size))), Width: size, Height: size, Mipmaps: 1}
}

// noColor has the framebuffer bound draw and read no colour.
func noColor() {
	C.pdx_no_color()
}

// depthBias offsets the depth of what is drawn by a slope and by steps of
// the depth buffer, as Direct3D's DepthBias and SlopeScaleDepthBias do, or
// stops offsetting it for zero and zero.
func depthBias(slope, steps float32) {
	C.pdx_depth_bias(C.float(slope), C.float(steps))
}
