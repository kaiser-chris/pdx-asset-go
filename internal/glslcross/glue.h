#ifndef GLSLCROSS_GLUE_H
#define GLSLCROSS_GLUE_H

#include <stdio.h>

// glslcross_sampler is a sampler of the GLSL that combines a texture and a
// sampler of the HLSL, by the names each has there.
typedef struct {
	char *name;
	char *image;
	char *sampler;
} glslcross_sampler;

enum {
	GLSLCROSS_OTHER,
	GLSLCROSS_FLOAT,
	GLSLCROSS_INT,
	GLSLCROSS_UINT,
	GLSLCROSS_BOOL,
};

// glslcross_uniform is one value of the constant buffers, by the name OpenGL
// knows it under, and its type: a scalar, a vector of vector_size, or a
// matrix of columns.
typedef struct {
	char *name;
	int kind;
	int vector_size;
	int columns;
} glslcross_uniform;

typedef struct {
	char *glsl;

	// failed_step and log say what went wrong when glsl is NULL.
	char *failed_step;
	char *log;
	char *warnings;

	glslcross_sampler *samplers;
	int sampler_count;

	glslcross_uniform *uniforms;
	int uniform_count;
	int uniform_capacity;
} glslcross_result;

int glslcross_initialize(void);
void glslcross_compile(const char *source, int pixel, const char *entry, int glsl_version, glslcross_result *result);
void glslcross_free(glslcross_result *result);

#endif
