package texture

import (
	"encoding/binary"
	"fmt"
)

// The cube maps the games light their models with: the environment map of
// every game, prefiltered for rough surfaces in its smaller levels.
//
// Each game stores its own differently, and all three are read:
//
//   - Victoria 3, as uncompressed eight bit pixels;
//   - Crusader Kings 3, compressed as DXT1;
//   - Europa Universalis 5, as half floats, D3D's A16B16G16R16F, for light
//     brighter than white.

const (
	// ddsCaps2Offset is where the second set of capabilities is, which says
	// whether a file is a cube map and which faces it holds.
	ddsCaps2Offset = 112

	ddsCubemap         = 0x200
	ddsCubemapAllFaces = 0xfc00

	// d3dA16B16G16R16F is the D3D format of four half floats, written in the
	// four character code of a file without a DX10 header.
	d3dA16B16G16R16F = 113

	dxgiR16G16B16A16Float = 10
)

// cubeFaces is how many faces a cube map has.
const cubeFaces = 6

// Cube is a decoded cube map, the way a graphics card takes it.
//
// Data holds Levels levels one after another, largest first, and every level
// the six faces in the order +X, -X, +Y, -Y, +Z, -Z, which is the order of a
// DDS file, and of OpenGL, which takes a cube map level by level.
type Cube struct {
	Size   int
	Format Format
	Levels int
	Data   []byte
}

// DecodeCube reads a DDS cube map.
//
// Compressed faces stay compressed and half floats stay half floats, with
// every level the file holds; eight bit pixels of any channel layout come
// out as RGBA8, every level of them.
func DecodeCube(data []byte) (*Cube, error) {
	header, err := readDDSHeader(data)
	if err != nil {
		return nil, err
	}

	caps2 := binary.LittleEndian.Uint32(data[ddsCaps2Offset:])
	if caps2&ddsCubemap == 0 || caps2&ddsCubemapAllFaces != ddsCubemapAllFaces {
		return nil, fmt.Errorf("DDS file is not a cube map of six faces")
	}

	if header.width != header.height {
		return nil, fmt.Errorf("DDS cube map has faces of %dx%d, which are not square", header.width, header.height)
	}

	format, levelSize, decodeLevel, err := cubeFormat(header)
	if err != nil {
		return nil, err
	}

	// A face holds its levels one after another, and the faces follow each
	// other.
	faceSize := 0
	for level := range header.levels {
		faceSize += levelSize(max(header.width>>level, 1))
	}

	if needed := cubeFaces * faceSize; len(data)-header.offset < needed {
		return nil, fmt.Errorf("DDS cube map is cut short: %d bytes, want %d", len(data)-header.offset, needed)
	}

	cube := &Cube{Size: header.width, Format: format, Levels: header.levels}

	for level := range header.levels {
		size := max(header.width>>level, 1)

		offset := 0
		for earlier := range level {
			offset += levelSize(max(header.width>>earlier, 1))
		}

		for face := range cubeFaces {
			start := header.offset + face*faceSize + offset

			decoded, err := decodeLevel(data, start, size)
			if err != nil {
				return nil, fmt.Errorf("face %d, level %d: %w", face, level, err)
			}

			cube.Data = append(cube.Data, decoded...)
		}
	}

	return cube, nil
}

// cubeFormat works out how a cube map's faces are stored: the format they
// come out in, how many bytes a level of a face of a size takes in the file,
// and how to decode one.
func cubeFormat(header ddsHeader) (Format, func(size int) int, func(data []byte, start, size int) ([]byte, error), error) {
	raw := func(format Format, bytesPerBlock func(size int) int) (Format, func(int) int, func([]byte, int, int) ([]byte, error), error) {
		return format, bytesPerBlock, func(data []byte, start, size int) ([]byte, error) {
			return data[start : start+bytesPerBlock(size)], nil
		}, nil
	}

	blocks := func(format Format) (Format, func(int) int, func([]byte, int, int) ([]byte, error), error) {
		return raw(format, func(size int) int { return levelsSize(size, size, 1, format) })
	}

	halfFloats := func(size int) int { return size * size * 8 }

	switch header.dxgi {
	case 0:
	case dxgiBC1Unorm, dxgiBC1UnormSRGB:
		return blocks(DXT1Alpha)
	case dxgiBC2Unorm, dxgiBC2UnormSRGB:
		return blocks(DXT3)
	case dxgiBC3Unorm, dxgiBC3UnormSRGB:
		return blocks(DXT5)
	case dxgiR16G16B16A16Float:
		return raw(RGBA16F, halfFloats)
	default:
		return 0, nil, nil, fmt.Errorf("DDS cube maps of format %d behind a DX10 header are not ones this reads", header.dxgi)
	}

	if header.flags&ddsFourCC != 0 {
		switch {
		case header.fourCC == "DXT1" && header.flags&ddsAlphaPixels != 0:
			return blocks(DXT1Alpha)
		case header.fourCC == "DXT1":
			return blocks(DXT1)
		case header.fourCC == "DXT3":
			return blocks(DXT3)
		case header.fourCC == "DXT5":
			return blocks(DXT5)
		case binary.LittleEndian.Uint32([]byte(header.fourCC)) == d3dA16B16G16R16F:
			return raw(RGBA16F, halfFloats)
		}

		return 0, nil, nil, fmt.Errorf("DDS cube maps of compression %q are not ones this reads", header.fourCC)
	}

	if header.flags&(ddsRGB|ddsLuminance) == 0 || header.bitCount%8 != 0 || header.bitCount < 8 || header.bitCount > 32 {
		return 0, nil, nil, fmt.Errorf("DDS cube map names neither a compression nor pixels this reads")
	}

	if header.flags&ddsAlphaPixels == 0 {
		header.masks[3] = 0
	}

	bytesPerPixel := header.bitCount / 8

	return RGBA8, func(size int) int { return size * size * bytesPerPixel }, func(data []byte, start, size int) ([]byte, error) {
		level := header
		level.width, level.height, level.levels, level.offset = size, size, 1, start

		decoded, err := decodeUncompressed(data, level)
		if err != nil {
			return nil, err
		}

		return decoded.Data, nil
	}, nil
}
