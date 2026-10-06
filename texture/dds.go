package texture

import (
	"encoding/binary"
	"fmt"
	"math/bits"
)

// Offsets into a DDS file. The header is a fixed layout, so the few fields that
// matter can be read straight out of it.
const (
	ddsMagicSize        = 4
	ddsHeaderSize       = 128 // magic plus the 124 byte header
	ddsExtendedSize     = 20  // the DX10 header that may follow
	ddsSizeOffset       = 4
	ddsHeightOffset     = 12
	ddsWidthOffset      = 16
	ddsMipCountOffset   = 28
	ddsPixelFlagsOffset = 80
	ddsFourCCOffset     = 84
	ddsBitCountOffset   = 88
	ddsMasksOffset      = 92
	ddsDXGIFormatOffs   = 128
)

// The size the header says it is, which every well formed file writes.
const ddsHeaderLength = 124

// Pixel format flags.
const (
	// ddsAlphaPixels is set when the pixels carry alpha: for DXT1 it means
	// the blocks use their one bit of alpha, for uncompressed pixels that the
	// alpha mask is in use.
	ddsAlphaPixels = 0x1

	// ddsAlphaOnly marks a file holding nothing but alpha.
	ddsAlphaOnly = 0x2

	// ddsFourCC says the format is named by its four character code.
	ddsFourCC = 0x4

	// ddsRGB and ddsLuminance say the pixels are stored uncompressed, as
	// colour or as grey.
	ddsRGB       = 0x40
	ddsLuminance = 0x20000
)

// MaxSize bounds the width and height of an image this package decodes. The
// games' largest textures are a few thousand pixels across; a header claiming
// more is broken, and believing it would only allocate memory for nothing.
const MaxSize = 16384

// DXGI formats that can follow a DX10 header, by their numbers in the DXGI
// format enum.
const (
	dxgiR8G8B8A8Unorm     = 28
	dxgiR8G8B8A8UnormSRGB = 29
	dxgiBC1Unorm          = 71
	dxgiBC1UnormSRGB      = 72
	dxgiBC2Unorm          = 74
	dxgiBC2UnormSRGB      = 75
	dxgiBC3Unorm          = 77
	dxgiBC3UnormSRGB      = 78
	dxgiB8G8R8A8Unorm     = 87
	dxgiB8G8R8X8Unorm     = 88
	dxgiB8G8R8A8UnormSRGB = 91
	dxgiB8G8R8X8UnormSRGB = 93
	dxgiBC7Unorm          = 98
	dxgiBC7UnormSRGB      = 99
)

var (
	ddsMagic  = [4]byte{'D', 'D', 'S', ' '}
	dx10Magic = [4]byte{'D', 'X', '1', '0'}
)

// ddsHeader is what a DDS file says about itself.
type ddsHeader struct {
	width, height, levels int

	flags    uint32
	fourCC   string
	bitCount int
	masks    [4]uint32 // red, green, blue, alpha

	// dxgi is the format of a file with a DX10 header, and zero otherwise.
	dxgi uint32

	// offset is where the pixels start.
	offset int
}

// readDDSHeader reads and checks the header of a DDS file.
func readDDSHeader(data []byte) (ddsHeader, error) {
	if len(data) < ddsHeaderSize || [4]byte(data[0:ddsMagicSize]) != ddsMagic {
		return ddsHeader{}, fmt.Errorf("not a DDS file")
	}

	value := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset : offset+4]) }

	if size := value(ddsSizeOffset); size != ddsHeaderLength {
		return ddsHeader{}, fmt.Errorf("DDS header says it is %d bytes, want %d", size, ddsHeaderLength)
	}

	header := ddsHeader{
		width:    int(value(ddsWidthOffset)),
		height:   int(value(ddsHeightOffset)),
		levels:   max(int(value(ddsMipCountOffset)), 1),
		flags:    value(ddsPixelFlagsOffset),
		fourCC:   string(data[ddsFourCCOffset : ddsFourCCOffset+4]),
		bitCount: int(value(ddsBitCountOffset)),
		offset:   ddsHeaderSize,
	}

	for index := range header.masks {
		header.masks[index] = value(ddsMasksOffset + index*4)
	}

	if header.width <= 0 || header.height <= 0 {
		return ddsHeader{}, fmt.Errorf("DDS image has no size")
	}

	if header.width > MaxSize || header.height > MaxSize {
		return ddsHeader{}, fmt.Errorf("DDS image claims to be %dx%d, more than the %d this reads", header.width, header.height, MaxSize)
	}

	// Every level is half the size of the one before it, down to a single
	// pixel, so an image has no more levels than that. A header that claims
	// more is believed only as far as that goes, rather than measured level
	// by level for millions of levels that cannot exist.
	header.levels = min(header.levels, bits.Len(uint(max(header.width, header.height))))

	if header.flags&ddsFourCC != 0 && [4]byte([]byte(header.fourCC)) == dx10Magic {
		if len(data) < ddsHeaderSize+ddsExtendedSize {
			return ddsHeader{}, fmt.Errorf("DDS file is cut short in its DX10 header")
		}

		header.dxgi = value(ddsDXGIFormatOffs)
		header.offset += ddsExtendedSize
	}

	return header, nil
}

// IsBC7 reports whether a DDS file is BC7 compressed, which no GPU raylib
// drives takes as it is, so it has to be decoded here.
func IsBC7(data []byte) bool {
	header, err := readDDSHeader(data)

	return err == nil && (header.dxgi == dxgiBC7Unorm || header.dxgi == dxgiBC7UnormSRGB)
}

// DecodeDDS reads a DDS file.
//
// The block compressions the GPU takes as they are, DXT1, DXT3 and DXT5 and
// their BC1 to BC3 spellings behind a DX10 header, come back compressed, with
// every mipmap level the file holds. Everything else comes back as RGBA8
// pixels of the largest level: BC7, decoded here, and uncompressed pixels of
// any channel layout, colour or grey, with or without alpha.
//
// A cubemap or an array of textures is read for its first image, which is
// stored first.
func DecodeDDS(data []byte) (*Image, error) {
	header, err := readDDSHeader(data)
	if err != nil {
		return nil, err
	}

	switch header.dxgi {
	case 0:
		// No DX10 header: the format is in the pixel format.

	case dxgiBC1Unorm, dxgiBC1UnormSRGB:
		return readBlocks(data, header, DXT1Alpha)
	case dxgiBC2Unorm, dxgiBC2UnormSRGB:
		return readBlocks(data, header, DXT3)
	case dxgiBC3Unorm, dxgiBC3UnormSRGB:
		return readBlocks(data, header, DXT5)

	case dxgiBC7Unorm, dxgiBC7UnormSRGB:
		pixels, err := DecodeBC7(data[header.offset:], header.width, header.height)
		if err != nil {
			return nil, err
		}

		return pixels.Image(), nil

	case dxgiR8G8B8A8Unorm, dxgiR8G8B8A8UnormSRGB:
		header.bitCount, header.masks = 32, [4]uint32{0xff, 0xff00, 0xff0000, 0xff000000}

		return decodeUncompressed(data, header)
	case dxgiB8G8R8A8Unorm, dxgiB8G8R8A8UnormSRGB:
		header.bitCount, header.masks = 32, [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}

		return decodeUncompressed(data, header)
	case dxgiB8G8R8X8Unorm, dxgiB8G8R8X8UnormSRGB:
		header.bitCount, header.masks = 32, [4]uint32{0xff0000, 0xff00, 0xff, 0}

		return decodeUncompressed(data, header)

	case dxgiR16G16B16A16Float:
		return halfFloatImage(data, header)

	default:
		return nil, fmt.Errorf("DDS format %d behind a DX10 header is not one this reads", header.dxgi)
	}

	if header.flags&ddsFourCC != 0 {
		if binary.LittleEndian.Uint32([]byte(header.fourCC)) == d3dA16B16G16R16F {
			return halfFloatImage(data, header)
		}

		switch header.fourCC {
		case "DXT1":
			format := DXT1
			if header.flags&ddsAlphaPixels != 0 {
				format = DXT1Alpha
			}

			return readBlocks(data, header, format)
		case "DXT3":
			return readBlocks(data, header, DXT3)
		case "DXT5":
			return readBlocks(data, header, DXT5)
		}

		return nil, fmt.Errorf("DDS compression %q is not one this reads", header.fourCC)
	}

	if header.flags&(ddsRGB|ddsLuminance|ddsAlphaOnly) != 0 {
		if header.flags&ddsAlphaPixels == 0 {
			// A mask the flags say is not in use may still hold rubbish.
			header.masks[3] = 0
		}

		return decodeUncompressed(data, header)
	}

	return nil, fmt.Errorf("DDS file names neither a compression nor uncompressed pixels")
}

// readBlocks reads the blocks of a block compressed file, with the smaller
// copies of the picture it carries.
//
// The levels are measured block by block, which is how the GPU measures them
// when they are uploaded. Rules of thumb, such as raylib's own of a third more
// than the largest level, come out a few bytes short for a texture with many
// levels, and the upload reads past the end of its buffer. The ordinary heap
// leaves memory there and nothing happens; the one Windows gives Store
// applications puts the end of the buffer at the end of a page, and the
// application crashes.
func readBlocks(data []byte, header ddsHeader, format Format) (*Image, error) {
	available := len(data) - header.offset

	// A file that holds fewer levels than it claims is read for the levels it
	// does hold, down to the largest one, which every file has.
	levels, size := 0, 0

	for level := range header.levels {
		next := size + levelsSize(max(header.width>>level, 1), max(header.height>>level, 1), 1, format)
		if next > available {
			break
		}

		levels, size = level+1, next
	}

	if levels == 0 {
		return nil, fmt.Errorf("DDS file is cut short: %d bytes of blocks, want %d", available, levelsSize(header.width, header.height, 1, format))
	}

	return &Image{
		Width:  header.width,
		Height: header.height,
		Format: format,
		Levels: levels,
		Data:   data[header.offset : header.offset+size],
	}, nil
}

// levelsSize is how many bytes the given number of mipmap levels take.
//
// Each level is half the size of the one before it, down to a single pixel,
// and blocks cover four by four pixels, so an edge that is not a multiple of
// four still takes a whole block.
func levelsSize(width, height, levels int, format Format) int {
	size := 0

	for level := range levels {
		across, down := max(width>>level, 1), max(height>>level, 1)
		size += (across + 3) / 4 * ((down + 3) / 4) * format.BlockSize()
	}

	return size
}

// decodeUncompressed reads the largest level of an uncompressed file, whatever
// the order and width of its channels, into RGBA8 pixels.
//
// The masks say which bits of a pixel hold which channel. A channel without a
// mask comes out as 0, alpha as fully opaque, and a grey file, which only
// sets the red mask, as the same value in all three colour channels.
func decodeUncompressed(data []byte, header ddsHeader) (*Image, error) {
	if header.bitCount%8 != 0 || header.bitCount < 8 || header.bitCount > 32 {
		return nil, fmt.Errorf("DDS pixels of %d bits are not ones this reads", header.bitCount)
	}

	bytesPerPixel := header.bitCount / 8
	rowSize := header.width * bytesPerPixel

	if needed := rowSize * header.height; len(data)-header.offset < needed {
		return nil, fmt.Errorf("DDS file is cut short: %d bytes of pixels, want %d", len(data)-header.offset, needed)
	}

	channels := [4]channel{}
	for index, mask := range header.masks {
		channels[index] = newChannel(mask)
	}

	grey := header.flags&ddsLuminance != 0

	pixels := NewPixels(header.width, header.height)
	source := data[header.offset:]

	for y := range header.height {
		row := source[y*rowSize:]

		for x := range header.width {
			var value uint32
			for index := range bytesPerPixel {
				value |= uint32(row[x*bytesPerPixel+index]) << (8 * index)
			}

			red := channels[0].read(value, 0)
			green, blue := channels[1].read(value, 0), channels[2].read(value, 0)

			if grey {
				green, blue = red, red
			}

			pixels.Set(x, y, red, green, blue, channels[3].read(value, 255))
		}
	}

	return pixels.Image(), nil
}

// channel reads one channel of an uncompressed pixel through its mask.
type channel struct {
	mask  uint32
	shift int
	width int
}

func newChannel(mask uint32) channel {
	if mask == 0 {
		return channel{}
	}

	shift := bits.TrailingZeros32(mask)

	return channel{mask: mask, shift: shift, width: bits.OnesCount32(mask >> shift)}
}

// read returns the channel's value scaled to eight bits, or fallback for a
// channel the file does not store.
func (c channel) read(pixel uint32, fallback byte) byte {
	if c.mask == 0 {
		return fallback
	}

	value := (pixel & c.mask) >> c.shift

	switch {
	case c.width == 8:
		return byte(value)
	case c.width > 8:
		return byte(value >> (c.width - 8))
	}

	// Fewer bits than eight are stretched over the whole range, so that the
	// largest value they hold is white rather than grey.
	largest := uint32(1)<<c.width - 1

	return byte((value*255 + largest/2) / largest)
}

// halfFloatImage reads the largest level of a picture of four half floats a
// pixel, such as the table Crusader Kings 3 tonemaps by.
func halfFloatImage(data []byte, header ddsHeader) (*Image, error) {
	size := header.width * header.height * 8
	if header.offset+size > len(data) {
		return nil, fmt.Errorf("DDS file of %dx%d half floats holds %d bytes of pixels, want %d", header.width, header.height, len(data)-header.offset, size)
	}

	return &Image{Width: header.width, Height: header.height, Format: RGBA16F, Levels: 1, Data: data[header.offset : header.offset+size]}, nil
}

// The formats of four half floats a channel: the D3D one, written in the
// four character code of a file without a DX10 header, and the DXGI one of
// a DX10 header.
const (
	d3dA16B16G16R16F      = 113
	dxgiR16G16B16A16Float = 10
)
