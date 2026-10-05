package render

import (
	"fmt"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// pixelFormats maps the formats of decoded images to raylib's.
//
// The numbers are raylib's own, from its PixelFormat enum in raylib.h. The
// constants raylib-go declares for the compressed formats cannot be used: its
// list leaves out the three half float formats that come before them, so
// rl.CompressedDxt5Rgba is 14, which raylib reads as DXT1, and every
// compressed format is off by three. A test reads raylib.h to keep these
// right.
var pixelFormats = map[texture.Format]rl.PixelFormat{
	texture.RGBA8:     7,  // PIXELFORMAT_UNCOMPRESSED_R8G8B8A8
	texture.DXT1:      14, // PIXELFORMAT_COMPRESSED_DXT1_RGB
	texture.DXT1Alpha: 15, // PIXELFORMAT_COMPRESSED_DXT1_RGBA
	texture.DXT3:      16, // PIXELFORMAT_COMPRESSED_DXT3_RGBA
	texture.DXT5:      17, // PIXELFORMAT_COMPRESSED_DXT5_RGBA
}

// UploadTexture hands a decoded image to the GPU, with the smaller copies of
// itself it is drawn with from afar: those the file brought, or for pixels
// decoded here, ones the GPU makes. It needs the OpenGL context.
func UploadTexture(decoded *texture.Image) (rl.Texture2D, error) {
	format, ok := pixelFormats[decoded.Format]
	if !ok {
		return rl.Texture2D{}, fmt.Errorf("textures of format %v cannot be uploaded", decoded.Format)
	}

	if decoded.Width <= 0 || decoded.Height <= 0 || decoded.Levels < 1 {
		return rl.Texture2D{}, fmt.Errorf("a texture of %dx%d with %d levels cannot be uploaded", decoded.Width, decoded.Height, decoded.Levels)
	}

	image, err := cImage(decoded, format)
	if err != nil {
		return rl.Texture2D{}, err
	}
	defer rl.UnloadImage(image)

	uploaded := rl.LoadTextureFromImage(image)
	if uploaded.ID == 0 {
		return rl.Texture2D{}, fmt.Errorf("the GPU did not take a %v texture of %dx%d", decoded.Format, decoded.Width, decoded.Height)
	}

	// Only pixels the GPU holds uncompressed can have their smaller copies
	// made by it; compressed files bring their own.
	if uploaded.Mipmaps <= 1 && decoded.Format == texture.RGBA8 {
		rl.GenTextureMipmaps(&uploaded)
	}

	if uploaded.Mipmaps > 1 {
		rl.SetTextureFilter(uploaded, rl.FilterTrilinear)
	} else {
		rl.SetTextureFilter(uploaded, rl.FilterBilinear)
	}

	return uploaded, nil
}

// cImage copies a decoded image into memory raylib owns.
//
// The copy is deliberate. A raylib image holding a Go slice would be a Go
// pointer inside a struct handed to C, which the cgo rules forbid, and the
// garbage collector would be free to move it out from under the upload.
// raylib-go offers no allocator of raylib's own, so the memory comes from an
// uncompressed image of whole blocks, which is always at least as large as the
// data, compressed or not.
func cImage(decoded *texture.Image, format rl.PixelFormat) (*rl.Image, error) {
	width := (decoded.Width + 3) / 4 * 4
	height := (decoded.Height + 3) / 4 * 4

	image := rl.GenImageColor(width, height, rl.Blank)
	if image == nil || image.Data == nil {
		return nil, fmt.Errorf("could not allocate a %dx%d image", decoded.Width, decoded.Height)
	}

	capacity := width * height * 4

	if len(decoded.Data) > capacity {
		rl.UnloadImage(image)

		return nil, fmt.Errorf("a %v texture of %dx%d with %d levels holds %d bytes, more than its %d", decoded.Format, decoded.Width, decoded.Height, decoded.Levels, len(decoded.Data), capacity)
	}

	copy(unsafe.Slice((*byte)(image.Data), capacity), decoded.Data)

	image.Width = int32(decoded.Width)
	image.Height = int32(decoded.Height)
	image.Mipmaps = int32(decoded.Levels)
	image.Format = format

	return image, nil
}

// Neutral stand ins for textures a part does not have.
var (
	// whitePixel draws the colour the rest of the material gives.
	whitePixel = [4]byte{255, 255, 255, 255}

	// flatNormal is a normal map pointing straight out of the surface: x and
	// y half way, in the channels the games keep them in.
	flatNormal = [4]byte{128, 128, 255, 255}

	// plainProperties is a material of no subsurface scattering, some
	// specular, no metal and a fairly rough surface.
	plainProperties = [4]byte{0, 128, 0, 190}
)

// uploadPixel uploads a texture of one pixel.
func uploadPixel(color [4]byte) (rl.Texture2D, error) {
	return UploadTexture(&texture.Image{Width: 1, Height: 1, Format: texture.RGBA8, Levels: 1, Data: color[:]})
}
