// The glue between Go and the C interfaces of glslang and SPIRV-Cross: one
// call that takes HLSL and returns GLSL, so that Go crosses into C once per
// stage and never holds on to an object of either library.

#include <stdlib.h>
#include <stdbool.h>
#include <string.h>

#include "glue.h"

#include "glslang/glslang/Include/glslang_c_interface.h"
#include "glslang/glslang/Public/resource_limits_c.h"
#include "spirvcross/spirv_cross_c.h"

static char *copy(const char *text) {
	if (text == NULL) {
		text = "";
	}

	size_t length = strlen(text);
	char *copied = malloc(length + 1);
	memcpy(copied, text, length + 1);

	return copied;
}

static void fail(glslcross_result *result, const char *step, const char *log) {
	result->failed_step = copy(step);
	result->log = copy(log);
}

static void spvc_error_log(void *userdata, const char *error) {
	glslcross_result *result = userdata;

	if (result->log == NULL) {
		result->log = copy(error);
	}
}

// add_uniform records one uniform the caller sets: a plain value, or one
// element of an array of them.
static void add_uniform(glslcross_result *result, const char *name, spvc_type type) {
	if (result->uniform_count == result->uniform_capacity) {
		result->uniform_capacity = result->uniform_capacity == 0 ? 64 : result->uniform_capacity * 2;
		result->uniforms = realloc(result->uniforms, result->uniform_capacity * sizeof(glslcross_uniform));
	}

	glslcross_uniform *uniform = &result->uniforms[result->uniform_count++];

	uniform->name = copy(name);
	uniform->vector_size = (int)spvc_type_get_vector_size(type);
	uniform->columns = (int)spvc_type_get_columns(type);

	switch (spvc_type_get_basetype(type)) {
	case SPVC_BASETYPE_FP32:
		uniform->kind = GLSLCROSS_FLOAT;
		break;
	case SPVC_BASETYPE_INT32:
		uniform->kind = GLSLCROSS_INT;
		break;
	case SPVC_BASETYPE_UINT32:
		uniform->kind = GLSLCROSS_UINT;
		break;
	case SPVC_BASETYPE_BOOLEAN:
		uniform->kind = GLSLCROSS_BOOL;
		break;
	default:
		uniform->kind = GLSLCROSS_OTHER;
	}
}

// add_uniforms records the uniforms below a name, down through structs and
// arrays, under the names OpenGL gives them: cb_PdxCamera.CameraPosition,
// cb_Lights.Colors[3].
static void add_uniforms(spvc_compiler compiler, glslcross_result *result, const char *name, spvc_type_id type_id) {
	spvc_type type = spvc_compiler_get_type_handle(compiler, type_id);

	unsigned dimensions = spvc_type_get_num_array_dimensions(type);
	if (dimensions > 0) {
		// The outermost dimension is the last; arrays of arrays are not
		// something the shaders declare.
		unsigned length = spvc_type_get_array_dimension(type, dimensions - 1);
		spvc_type_id element = spvc_type_get_base_type_id(type);

		for (unsigned index = 0; index < length; index++) {
			char element_name[512];
			snprintf(element_name, sizeof(element_name), "%s[%u]", name, index);

			if (dimensions > 1) {
				add_uniform(result, element_name, type);
				continue;
			}

			spvc_type element_type = spvc_compiler_get_type_handle(compiler, element);
			if (spvc_type_get_basetype(element_type) == SPVC_BASETYPE_STRUCT) {
				add_uniforms(compiler, result, element_name, element);
			} else {
				add_uniform(result, element_name, element_type);
			}
		}

		return;
	}

	if (spvc_type_get_basetype(type) != SPVC_BASETYPE_STRUCT) {
		add_uniform(result, name, type);

		return;
	}

	unsigned members = spvc_type_get_num_member_types(type);

	for (unsigned index = 0; index < members; index++) {
		char member_name[512];
		snprintf(member_name, sizeof(member_name), "%s.%s", name, spvc_compiler_get_member_name(compiler, type_id, index));

		add_uniforms(compiler, result, member_name, spvc_type_get_member_type(type, index));
	}
}

// name_resources gives what the stages pass on and what the caller sets
// names of their own, and records the uniforms.
//
// OpenGL 3.30 matches the outputs of the vertex stage to the inputs of the
// pixel stage by name, and SPIRV-Cross names them after the variables of the
// HLSL, which differ between the stages: both are named after the location
// glslang gave them, which is the same at both ends. The constant buffers,
// which become plain uniforms, are named cb_ and the buffer's name.
static void name_resources(spvc_compiler compiler, int pixel, glslcross_result *result) {
	spvc_resources resources = NULL;
	if (spvc_compiler_create_shader_resources(compiler, &resources) != SPVC_SUCCESS) {
		return;
	}

	const spvc_reflected_resource *list = NULL;
	size_t count = 0;

	spvc_resources_get_resource_list_for_type(resources, pixel ? SPVC_RESOURCE_TYPE_STAGE_INPUT : SPVC_RESOURCE_TYPE_STAGE_OUTPUT, &list, &count);

	for (size_t index = 0; index < count; index++) {
		char name[64];
		snprintf(name, sizeof(name), "pdx_varying_%u", spvc_compiler_get_decoration(compiler, list[index].id, SpvDecorationLocation));
		spvc_compiler_set_name(compiler, list[index].id, name);

		// A whole number is not interpolated: Vulkan marks it flat where the
		// pixel stage reads it, and OpenGL 3.30 wants the vertex stage to
		// say the same.
		spvc_type type = spvc_compiler_get_type_handle(compiler, list[index].type_id);
		spvc_basetype base = spvc_type_get_basetype(type);

		if (!pixel && (base == SPVC_BASETYPE_INT32 || base == SPVC_BASETYPE_UINT32 || base == SPVC_BASETYPE_BOOLEAN)) {
			spvc_compiler_set_decoration(compiler, list[index].id, SpvDecorationFlat, 0);
		}
	}

	spvc_resources_get_resource_list_for_type(resources, SPVC_RESOURCE_TYPE_UNIFORM_BUFFER, &list, &count);

	for (size_t index = 0; index < count; index++) {
		const char *block = spvc_compiler_get_name(compiler, list[index].base_type_id);
		if (block == NULL || block[0] == '\0') {
			block = list[index].name;
		}

		// HLSL puts the uniforms outside a constant buffer in one called
		// $Global.
		char name[256] = "cb_";
		size_t length = strlen(name);

		for (const char *c = block; *c != '\0' && length < sizeof(name) - 1; c++) {
			if ((*c >= 'a' && *c <= 'z') || (*c >= 'A' && *c <= 'Z') || (*c >= '0' && *c <= '9') || *c == '_') {
				name[length++] = *c;
			}
		}

		name[length] = '\0';

		spvc_compiler_set_name(compiler, list[index].id, name);
		add_uniforms(compiler, result, name, list[index].base_type_id);
	}
}

int glslcross_initialize(void) {
	return glslang_initialize_process();
}

// glslcross_compile compiles one stage of HLSL to SPIR-V with glslang, then
// the SPIR-V to GLSL with SPIRV-Cross. The result holds the GLSL, or the step
// that failed and its log, and the combined image samplers, which the caller
// frees with glslcross_free.
void glslcross_compile(const char *source, int pixel, const char *entry, int glsl_version, glslcross_result *result) {
	memset(result, 0, sizeof(*result));

	glslang_stage_t stage = pixel ? GLSLANG_STAGE_FRAGMENT : GLSLANG_STAGE_VERTEX;

	glslang_input_t input = {
		.language = GLSLANG_SOURCE_HLSL,
		.stage = stage,
		.client = GLSLANG_CLIENT_VULKAN,
		.client_version = GLSLANG_TARGET_VULKAN_1_0,
		.target_language = GLSLANG_TARGET_SPV,
		.target_language_version = GLSLANG_TARGET_SPV_1_0,
		.code = source,
		.default_version = 100,
		.default_profile = GLSLANG_NO_PROFILE,
		.force_default_version_and_profile = 0,
		.forward_compatible = 0,
		.messages = GLSLANG_MSG_SPV_RULES_BIT | GLSLANG_MSG_VULKAN_RULES_BIT,
		.resource = glslang_default_resource(),
	};

	glslang_shader_t *shader = glslang_shader_create(&input);
	glslang_shader_set_entry_point(shader, entry);
	glslang_shader_set_source_entry_point(shader, entry);
	glslang_shader_set_options(shader, GLSLANG_SHADER_AUTO_MAP_BINDINGS | GLSLANG_SHADER_AUTO_MAP_LOCATIONS);

	if (!glslang_shader_preprocess(shader, &input)) {
		fail(result, "preprocess", glslang_shader_get_info_log(shader));
		glslang_shader_delete(shader);

		return;
	}

	if (!glslang_shader_parse(shader, &input)) {
		fail(result, "parse", glslang_shader_get_info_log(shader));
		glslang_shader_delete(shader);

		return;
	}

	glslang_program_t *program = glslang_program_create();
	glslang_program_add_shader(program, shader);

	if (!glslang_program_link(program, GLSLANG_MSG_SPV_RULES_BIT | GLSLANG_MSG_VULKAN_RULES_BIT)) {
		fail(result, "link", glslang_program_get_info_log(program));
		glslang_program_delete(program);
		glslang_shader_delete(shader);

		return;
	}

	if (!glslang_program_map_io(program)) {
		fail(result, "map", glslang_program_get_info_log(program));
		glslang_program_delete(program);
		glslang_shader_delete(shader);

		return;
	}

	// The optimizer is off by default, and with it the legalization HLSL
	// needs: inlining the functions and taking apart the structs that hold
	// a texture and its sampler. Nothing else is optimized.
	glslang_spv_options_t spv_options = {0};
	spv_options.disable_optimizer = false;

	glslang_program_SPIRV_generate_with_options(program, stage, &spv_options);

	size_t words = glslang_program_SPIRV_get_size(program);
	SpvId *spirv = malloc(words * sizeof(SpvId));
	glslang_program_SPIRV_get(program, spirv);

	const char *messages = glslang_program_SPIRV_get_messages(program);
	if (messages != NULL && messages[0] != '\0') {
		result->warnings = copy(messages);
	}

	glslang_program_delete(program);
	glslang_shader_delete(shader);

	spvc_context context = NULL;
	spvc_context_create(&context);
	spvc_context_set_error_callback(context, spvc_error_log, result);

	spvc_parsed_ir ir = NULL;
	spvc_compiler compiler = NULL;
	spvc_compiler_options options = NULL;
	const char *glsl = NULL;

	if (spvc_context_parse_spirv(context, spirv, words, &ir) != SPVC_SUCCESS ||
		spvc_context_create_compiler(context, SPVC_BACKEND_GLSL, ir, SPVC_CAPTURE_MODE_TAKE_OWNERSHIP, &compiler) != SPVC_SUCCESS ||
		spvc_compiler_create_compiler_options(compiler, &options) != SPVC_SUCCESS) {
		fail(result, "cross", result->log);
		free(spirv);
		spvc_context_destroy(context);

		return;
	}

	free(spirv);

	spvc_compiler_options_set_uint(options, SPVC_COMPILER_OPTION_GLSL_VERSION, glsl_version);
	spvc_compiler_options_set_bool(options, SPVC_COMPILER_OPTION_GLSL_ES, SPVC_FALSE);
	spvc_compiler_options_set_bool(options, SPVC_COMPILER_OPTION_GLSL_VULKAN_SEMANTICS, SPVC_FALSE);
	spvc_compiler_options_set_bool(options, SPVC_COMPILER_OPTION_GLSL_ENABLE_420PACK_EXTENSION, SPVC_FALSE);

	// raylib sets uniforms one by one and binds no uniform buffers, so the
	// constant buffers become plain uniforms of a struct type.
	spvc_compiler_options_set_bool(options, SPVC_COMPILER_OPTION_GLSL_EMIT_UNIFORM_BUFFER_AS_PLAIN_UNIFORMS, SPVC_TRUE);
	spvc_compiler_install_compiler_options(compiler, options);

	// HLSL samples a texture through a sampler of its own; OpenGL samples
	// through one object that is both. Every pair the code uses becomes one,
	// and a texture the code only reads texel by texel, with Load, is paired
	// with a sampler of SPIRV-Cross's own.
	spvc_variable_id dummy = 0;
	spvc_compiler_build_dummy_sampler_for_combined_images(compiler, &dummy);

	if (spvc_compiler_build_combined_image_samplers(compiler) != SPVC_SUCCESS) {
		fail(result, "cross", result->log);
		spvc_context_destroy(context);

		return;
	}

	const spvc_combined_image_sampler *combined = NULL;
	size_t count = 0;
	spvc_compiler_get_combined_image_samplers(compiler, &combined, &count);

	result->samplers = calloc(count == 0 ? 1 : count, sizeof(glslcross_sampler));
	result->sampler_count = (int)count;

	for (size_t index = 0; index < count; index++) {
		const char *image = spvc_compiler_get_name(compiler, combined[index].image_id);
		const char *sampler = spvc_compiler_get_name(compiler, combined[index].sampler_id);

		// Named after the texture and the sampler, which both stages name
		// alike, so that a pair both stages sample is one sampler of the
		// program once it is linked: DiffuseMap._Texture with
		// DiffuseMap._Sampler is pdx_DiffuseMap_Texture_DiffuseMap_Sampler.
		// GLSL reserves names with two underscores in a row, so there are
		// none.
		size_t length = strlen(image) + strlen(sampler) + 16;
		char *name = malloc(length);
		snprintf(name, length, "pdx_%s_%s", image, sampler);

		size_t out = 0;
		for (size_t in = 0; name[in] != '\0'; in++) {
			char c = name[in];
			if (!((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))) {
				c = '_';
			}

			if (c == '_' && out > 0 && name[out - 1] == '_') {
				continue;
			}

			name[out++] = c;
		}

		while (out > 0 && name[out - 1] == '_') {
			out--;
		}

		name[out] = '\0';

		spvc_compiler_set_name(compiler, combined[index].combined_id, name);

		result->samplers[index].name = name;
		result->samplers[index].image = copy(image);
		result->samplers[index].sampler = copy(sampler);
	}

	name_resources(compiler, pixel, result);

	if (spvc_compiler_compile(compiler, &glsl) != SPVC_SUCCESS) {
		fail(result, "cross", result->log);
		spvc_context_destroy(context);

		return;
	}

	result->glsl = copy(glsl);
	free(result->log);
	result->log = NULL;

	spvc_context_destroy(context);
}

void glslcross_free(glslcross_result *result) {
	free(result->glsl);
	free(result->failed_step);
	free(result->log);
	free(result->warnings);

	for (int index = 0; index < result->sampler_count; index++) {
		free(result->samplers[index].name);
		free(result->samplers[index].image);
		free(result->samplers[index].sampler);
	}

	free(result->samplers);

	for (int index = 0; index < result->uniform_count; index++) {
		free(result->uniforms[index].name);
	}

	free(result->uniforms);
	memset(result, 0, sizeof(*result));
}
