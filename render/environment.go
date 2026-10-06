package render

import (
	"fmt"
	"math"
	"strings"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/environment"
	"github.com/kaiser-chris/pdx-asset-go/shader"
	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// The environment the games light their models with: the constants of
// JominiEnvironment, set from the game's environment file, and its
// environment map, which the samplers referring to JominiEnvironmentMap read.
//
// The models are mirrored across x on their way into OpenGL's coordinates,
// and with them the directions of the file: the direction to the sun, and
// every lookup into the environment map, which the shaders turn by
// CubemapYRotation before they make it, so that it holds the mirror as well
// as the turn.

// environmentMapRef is what the engine binds its environment map to.
const environmentMapRef = "JominiEnvironmentMap"

// SetEnvironment lights the games' effects with an environment and its
// environment map, which may be nil for none. A part drawn with the
// renderer's own shader is not affected. It needs the OpenGL context.
func (r *Renderer) SetEnvironment(lighting *environment.Environment, cube *texture.Cube) error {
	r.unloadEnvironment()
	r.releasePost()
	r.environment = lighting

	if cube == nil {
		return nil
	}

	uploaded, err := uploadCube(cube)
	if err != nil {
		return err
	}

	r.environmentMap = uploaded
	r.environmentSRGB = cube.Format != texture.RGBA16F

	return nil
}

func (r *Renderer) unloadEnvironment() {
	if r.environmentMap.ID != 0 {
		rl.UnloadTexture(r.environmentMap)
		r.environmentMap = rl.Texture2D{}
	}

	r.environment = nil
}

// cubeFor is the cube map a cube sampler of an effect reads, and whether it
// is converted from sRGB as it is sampled: the environment map for the one
// the engine binds it to, the file the sampler names for itself, such as the
// environment maps of Crusader Kings 3's map objects, and white otherwise.
//
// The environment map is converted when it is a texture of eight bits a
// channel; one of half floats holds linear light. Crusader Kings 3 shows
// it: its environment file names environment_terrain_sunny.dds, which its
// own samplers of that file mark sRGB = yes, and lights by it twenty times
// over, which only the converted, darker light makes sense of.
func (r *Renderer) cubeFor(bound shader.Texture) (rl.Texture2D, bool) {
	if bound.Sampler == nil {
		return r.whiteCube, false
	}

	if bound.Sampler.Ref == environmentMapRef {
		// An environment without a map gives no light from around, as
		// Victoria 3 draws its environment_greyscale.txt, which names none.
		if r.environmentMap.ID == 0 {
			return r.blackCube, false
		}

		return r.environmentMap, r.environmentSRGB
	}

	if bound.Sampler.File != "" {
		return r.fileCube(bound.Sampler.File), bound.Sampler.SRGB
	}

	return r.whiteCube, false
}

// fileCube uploads a cube map a sampler names for itself, once. One that
// cannot be read is white.
func (r *Renderer) fileCube(name string) rl.Texture2D {
	key := "cube:" + name
	if found, ok := r.files[key]; ok {
		return found
	}

	if r.files == nil {
		r.files = map[string]rl.Texture2D{}
	}

	uploaded := r.whiteCube

	if data, err := r.source.ReadFile(name); err == nil {
		if cube, err := texture.DecodeCube(data); err == nil {
			if done, err := uploadCube(cube); err == nil {
				uploaded = done
			}
		}
	}

	r.files[key] = uploaded

	return uploaded
}

// applyEnvironment sets the constants of the environment on an effect. What
// the file does not set keeps its basic default.
func (r *Renderer) applyEnvironment(compiled *effect) {
	if r.environment == nil {
		return
	}

	for name, values := range r.environment.Constants {
		location, ok := compiled.environment[name]
		if !ok {
			continue
		}

		values = append([]float32(nil), values...)

		if isDirection(name) && len(values) == 3 {
			// Mirrored like the models, and for the sun a direction of
			// length one, which the file does not write it as.
			values[0] = -values[0]

			if name == "ToSunDir" {
				length := float32(math.Sqrt(float64(values[0]*values[0] + values[1]*values[1] + values[2]*values[2])))
				if length > 0 {
					for index := range values {
						values[index] /= length
					}
				}
			}
		}

		switch len(values) {
		case 1:
			rl.SetShaderValue(compiled.shader, location, values, rl.ShaderUniformFloat)
		case 2:
			rl.SetShaderValue(compiled.shader, location, values, rl.ShaderUniformVec2)
		case 3:
			rl.SetShaderValue(compiled.shader, location, values, rl.ShaderUniformVec3)
		case 4:
			rl.SetShaderValue(compiled.shader, location, values, rl.ShaderUniformVec4)
		}
	}

	// The constants the graphics defines set.
	for member, constant := range compiled.shared {
		if values, ok := r.environment.Defines.Constant(member); ok && len(values) == constant.uniform.VectorSize {
			setFloats(compiled.shader, constant.location, values)
		}
	}

	if location, ok := compiled.environment["CubemapYRotation"]; ok {
		rl.SetShaderValueMatrix(compiled.shader, location, rl.MatrixTranspose(cubemapRotation(r.environment.CubemapYRotation)))
	}
}

// isDirection reports whether a constant of the environment is a direction,
// which is mirrored with the models.
func isDirection(name string) bool {
	return strings.HasSuffix(name, "Dir") || strings.HasSuffix(name, "Direction")
}

// cubemapRotation is the matrix the shaders turn a lookup into the
// environment map by: the turn about the vertical the file asks for, after
// mirroring the direction back across x into the game's coordinates.
func cubemapRotation(degrees float32) rl.Matrix {
	angle := float64(degrees) * math.Pi / 180
	c, s := float32(math.Cos(angle)), float32(math.Sin(angle))

	// The turn about y, times the mirror across x, as rows: raylib names
	// the element of row r and column c M(4c+r).
	return rl.Matrix{
		M0: -c, M4: 0, M8: s, M12: 0,
		M1: 0, M5: 1, M9: 0, M13: 0,
		M2: s, M6: 0, M10: c, M14: 0,
		M3: 0, M7: 0, M11: 0, M15: 1,
	}
}

// uploadCube hands a cube map to the GPU, with its smaller levels.
//
// raylib takes a cube map as its six faces one above the other, and works
// out the size of every level of that column as if it were one picture. For
// pixels that is the size of the six faces; for compressed blocks it is not
// once a face is smaller than a block, so those last levels are left out.
func uploadCube(cube *texture.Cube) (rl.Texture2D, error) {
	format, ok := pixelFormats[cube.Format]
	if !ok {
		return rl.Texture2D{}, fmt.Errorf("cube maps of format %v cannot be uploaded", cube.Format)
	}

	levels, size := 0, 0

	for level := range cube.Levels {
		face := max(cube.Size>>level, 1)
		if cube.Format.Compressed() && face < 4 && level > 0 {
			break
		}

		levels, size = level+1, size+6*faceSize(cube.Format, face)
	}

	if size > len(cube.Data) {
		return rl.Texture2D{}, fmt.Errorf("a cube map of %d across holds %d bytes, want %d", cube.Size, len(cube.Data), size)
	}

	// Memory raylib owns, for the reason cImage gives: six faces of the
	// largest level in pixels of four bytes are a column of the size times
	// six, and the levels and wider pixels fit in three more.
	image := rl.GenImageColor(cube.Size, cube.Size*6*3, rl.Blank)
	if image == nil || image.Data == nil {
		return rl.Texture2D{}, fmt.Errorf("could not allocate a cube map of %d across", cube.Size)
	}
	defer rl.UnloadImage(image)

	capacity := cube.Size * cube.Size * 6 * 3 * 4
	if size > capacity {
		return rl.Texture2D{}, fmt.Errorf("a cube map of %d across takes %d bytes, more than its %d", cube.Size, size, capacity)
	}

	copy(unsafe.Slice((*byte)(image.Data), capacity), cube.Data[:size])

	image.Width = int32(cube.Size)
	image.Height = int32(cube.Size * 6)
	image.Mipmaps = int32(levels)
	image.Format = format

	uploaded := rl.LoadTextureCubemap(image, rl.CubemapLayoutLineVertical)
	if uploaded.ID == 0 {
		return rl.Texture2D{}, fmt.Errorf("the GPU did not take a %v cube map of %d across", cube.Format, cube.Size)
	}

	setCubeLevels(uploaded, levels)

	return uploaded, nil
}

// faceSize is how many bytes one face of a size takes in a format.
func faceSize(format texture.Format, size int) int {
	switch format {
	case texture.RGBA8:
		return size * size * 4
	case texture.RGBA16F:
		return size * size * 8
	}

	return (size + 3) / 4 * ((size + 3) / 4) * format.BlockSize()
}
