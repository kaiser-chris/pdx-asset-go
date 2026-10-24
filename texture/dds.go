package texture

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math/bits"

	"github.com/kaiser-chris/dds-go"
)

// DDS files are read by dds-go, which reads every format and layout DirectX
// has: this package only decides what to keep of them. The block
// compressions a GPU takes as they are stay compressed, half floats stay half
// floats, and everything else is decoded to RGBA8.

// MaxSize bounds the width and height of an image this package decodes. The
// games' largest textures are a few thousand pixels across; a header claiming
// more is broken, and believing it would only allocate memory for nothing.
const MaxSize = 16384

// ddsMipCountOffset is where a DDS header keeps its count of mipmap levels,
// which a file that holds fewer is read again with.
const ddsMipCountOffset = 28

// cubemapFaceCount is how many faces a cubemap has.
const cubemapFaceCount = 6

// DecodeDDS reads a DDS file.
//
// The block compressions the GPU takes as they are, DXT1, DXT3 and DXT5 and
// their BC1 to BC3 spellings behind a DX10 header, come back compressed, with
// every mipmap level the file holds. Four half floats a pixel come back as
// RGBA16F, the largest level. Everything else comes back as RGBA8 pixels of
// the largest level: BC4 to BC7, the HDR and signed formats rounded to eight
// bits, and uncompressed pixels of any channel layout, colour or grey, with or
// without alpha.
//
// A cubemap or an array of textures is read for its first image, which is
// stored first.
func DecodeDDS(data []byte) (*Image, error) {
	file, err := parseDDS(data)
	if err != nil {
		return nil, err
	}

	return decodeImage(file, 0)
}

// parseDDS parses a DDS file, which a header claiming more mipmap levels than
// the file holds does not fail: it is read for the levels it does hold, down
// to the largest one, which every file has. A count beyond what halving the
// image goes to is believed only that far.
func parseDDS(data []byte) (*dds.File, error) {
	if len(data) < 4 || string(data[:4]) != "DDS " {
		return nil, fmt.Errorf("not a DDS file")
	}

	file, err := dds.Parse(data)
	if err == nil || errors.Is(err, dds.ErrNotDDS) || errors.Is(err, dds.ErrUnsupported) || len(data) < ddsMipCountOffset+4 {
		return file, err
	}

	header, headerErr := dds.ReadHeader(bytes.NewReader(data))
	if headerErr != nil {
		return nil, err
	}

	claimed := min(int(header.Header.MipMapCount), bits.Len(uint(max(header.Width, header.Height, 1))))

	fewer := append([]byte(nil), data...)
	for levels := claimed - 1; levels >= 1; levels-- {
		binary.LittleEndian.PutUint32(fewer[ddsMipCountOffset:], uint32(levels))

		if file, fewerErr := dds.Parse(fewer); fewerErr == nil {
			return file, nil
		}
	}

	return nil, err
}

// decodeImage keeps what the GPU takes of one image of a DDS file, and decodes
// the rest.
func decodeImage(file *dds.File, index int) (*Image, error) {
	if format, ok := blockFormat(file); ok {
		var blocks []byte

		for level := range file.MipLevels {
			surface, err := file.Surface(index, level)
			if err != nil {
				return nil, err
			}

			blocks = append(blocks, surface.Data...)
		}

		return &Image{Width: file.Width, Height: file.Height, Format: format, Levels: file.MipLevels, Data: blocks}, nil
	}

	if file.Format == dds.R16G16B16A16Float {
		half, err := file.DecodeHalf(index, 0)
		if err != nil {
			return nil, err
		}

		return halfImage(half), nil
	}

	if file.Width > MaxSize || file.Height > MaxSize {
		return nil, fmt.Errorf("DDS image claims to be %dx%d, more than the %d this reads", file.Width, file.Height, MaxSize)
	}

	pixels, err := file.Decode(index, 0)
	if err != nil {
		return nil, err
	}

	return nrgbaImage(pixels), nil
}

// blockFormat is the format of the blocks of a file the GPU takes as they are.
// A DXT1 file without a DX10 header is read with its one bit of alpha when
// its header says the pixels have alpha; behind a DX10 header, where nothing
// says so, it always is.
func blockFormat(file *dds.File) (Format, bool) {
	switch file.Format {
	case dds.BC1Unorm, dds.BC1UnormSRGB:
		if file.DX10 == nil && file.Header.PixelFormat.Flags&ddsAlphaPixels == 0 {
			return DXT1, true
		}

		return DXT1Alpha, true
	case dds.BC2Unorm, dds.BC2UnormSRGB:
		return DXT3, true
	case dds.BC3Unorm, dds.BC3UnormSRGB:
		return DXT5, true
	}

	return 0, false
}

// ddsAlphaPixels is the flag of a legacy pixel format that says the pixels
// carry alpha.
const ddsAlphaPixels = 0x1

// halfImage lays out half floats as RGBA16F.
func halfImage(half *dds.HalfImage) *Image {
	width, height := half.Rect.Dx(), half.Rect.Dy()
	data := make([]byte, 0, width*height*8)

	for y := range height {
		row := half.Pix[y*half.Stride : y*half.Stride+width*4]
		for _, value := range row {
			data = binary.LittleEndian.AppendUint16(data, value)
		}
	}

	return &Image{Width: width, Height: height, Format: RGBA16F, Levels: 1, Data: data}
}

// nrgbaImage lays out pixels as RGBA8.
func nrgbaImage(pixels *image.NRGBA) *Image {
	width, height := pixels.Rect.Dx(), pixels.Rect.Dy()
	data := make([]byte, 0, width*height*4)

	for y := range height {
		data = append(data, pixels.Pix[y*pixels.Stride:y*pixels.Stride+width*4]...)
	}

	return &Image{Width: width, Height: height, Format: RGBA8, Levels: 1, Data: data}
}

// DecodeCubeDDS reads the six faces of a cubemap DDS file, in the order of the
// axes they face: +x, -x, +y, -y, +z, -z. Each face is read as DecodeDDS reads
// an image, with the mipmap levels the file holds for it where they are kept
// compressed.
//
// The games light their portraits with cubemaps of their surroundings, such
// as Victoria 3's castle_interior_01.dds, which is DXT1. A file that is no
// cubemap, or leaves a face out, is reported.
func DecodeCubeDDS(data []byte) ([cubemapFaceCount]*Image, error) {
	var faces [cubemapFaceCount]*Image

	file, err := parseDDS(data)
	if err != nil {
		return faces, err
	}

	if file.Dimension != dds.TextureCube {
		return faces, fmt.Errorf("DDS file is not a cubemap")
	}

	for face := range cubemapFaceCount {
		index, ok := file.FaceImage(0, dds.Face(face))
		if !ok {
			return faces, fmt.Errorf("DDS cubemap has no face %d", face+1)
		}

		faces[face], err = decodeImage(file, index)
		if err != nil {
			return faces, fmt.Errorf("face %d of the cubemap: %w", face+1, err)
		}
	}

	return faces, nil
}
