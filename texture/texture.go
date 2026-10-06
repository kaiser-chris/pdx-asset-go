// Package texture decodes the image files the Paradox games ship.
//
// It reads every format the games' models use:
//
//   - DDS with DXT1, DXT3 or DXT5 blocks, or their BC1 to BC3 spellings behind
//     a DX10 header, which stay compressed, since the GPU takes them as they
//     are, with every mipmap level the file holds.
//   - DDS with BC7 blocks, which no GPU raylib drives takes, decoded to
//     pixels.
//   - Uncompressed DDS of any channel layout, such as the 32 bit pixels of
//     the portraits' diffuse and normal maps.
//   - Targa and PNG.
//
// The package is plain Go: it knows nothing about raylib or OpenGL, so it can
// be tested on its own, and handing an Image to the GPU is the render
// package's job. Decoding never trusts a header further than the file backs
// it up: a size that claims more pixels than the file holds is refused before
// anything is allocated for it.
package texture

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"path/filepath"
	"strings"
)

// Format is how the pixels of an Image are stored.
type Format int

const (
	// RGBA8 is four bytes per pixel in red, green, blue, alpha order, with
	// the first row at the top.
	RGBA8 Format = iota + 1

	// The block compressions, each storing a block of four by four pixels.
	DXT1
	DXT1Alpha
	DXT3
	DXT5

	// RGBA16F is four half floats per pixel, in red, green, blue, alpha
	// order: light brighter than white, as the cube maps of Europa
	// Universalis 5 hold it.
	RGBA16F
)

func (f Format) String() string {
	switch f {
	case RGBA8:
		return "RGBA8"
	case DXT1:
		return "DXT1"
	case DXT1Alpha:
		return "DXT1 with alpha"
	case DXT3:
		return "DXT3"
	case DXT5:
		return "DXT5"
	case RGBA16F:
		return "RGBA16F"
	}

	return "unknown"
}

// Compressed reports whether the format is a block compression.
func (f Format) Compressed() bool {
	return f >= DXT1 && f <= DXT5
}

// BlockSize is how many bytes one block of four by four pixels takes, for a
// block compression.
func (f Format) BlockSize() int {
	switch f {
	case DXT1, DXT1Alpha:
		return 8
	case DXT3, DXT5:
		return 16
	}

	return 0
}

// Image is a decoded image, the way a graphics card takes it.
//
// Data holds Levels mipmap levels one after another, largest first, which is
// the order a DDS file and the graphics card both keep them in. Decoded pixels
// only ever hold the largest level; the smaller ones can be made on the GPU.
type Image struct {
	Width, Height int
	Format        Format
	Levels        int
	Data          []byte
}

// Decode reads an image file.
//
// name is only used for its extension, so the bytes can come from anywhere.
func Decode(name string, data []byte) (*Image, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}

	extension := strings.ToLower(filepath.Ext(name))

	var (
		decoded *Image
		err     error
	)

	switch extension {
	case ".dds":
		decoded, err = DecodeDDS(data)

	case ".tga":
		var pixels *Pixels

		pixels, err = DecodeTGA(data)
		if err == nil {
			decoded = pixels.Image()
		}

	case ".png":
		decoded, err = decodePNG(data)

	default:
		err = fmt.Errorf("%q files are not ones this reads", extension)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	return decoded, nil
}

// decodePNG reads a PNG file into RGBA8 pixels.
func decodePNG(data []byte) (*Image, error) {
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	if config.Width > MaxSize || config.Height > MaxSize {
		return nil, fmt.Errorf("PNG image claims to be %dx%d, more than the %d this reads", config.Width, config.Height, MaxSize)
	}

	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := picture.Bounds()

	// Drawing into a non-premultiplied image keeps the colour of a pixel that
	// is partly transparent, which is what a texture holds.
	rgba := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), picture, bounds.Min, draw.Src)

	return &Image{Width: bounds.Dx(), Height: bounds.Dy(), Format: RGBA8, Levels: 1, Data: rgba.Pix}, nil
}

// Pixels is a decoded image, four bytes per pixel in red, green, blue, alpha
// order, with the first row at the top.
type Pixels struct {
	Width  int
	Height int
	Data   []byte
}

// NewPixels allocates an image of the given size.
func NewPixels(width, height int) *Pixels {
	return &Pixels{Width: width, Height: height, Data: make([]byte, width*height*4)}
}

// At returns the offset of a pixel in Data.
func (p *Pixels) At(x, y int) int {
	return (y*p.Width + x) * 4
}

// Set writes one pixel.
func (p *Pixels) Set(x, y int, r, g, b, a byte) {
	offset := p.At(x, y)
	p.Data[offset] = r
	p.Data[offset+1] = g
	p.Data[offset+2] = b
	p.Data[offset+3] = a
}

// Image turns the pixels into an Image of one level.
func (p *Pixels) Image() *Image {
	return &Image{Width: p.Width, Height: p.Height, Format: RGBA8, Levels: 1, Data: p.Data}
}
