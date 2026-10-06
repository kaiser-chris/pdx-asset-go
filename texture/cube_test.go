package texture

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// cubeHeader is the header of a cube map of all six faces.
func cubeHeader(file ddsFile) []byte {
	header := file.header()
	binary.LittleEndian.PutUint32(header[ddsCaps2Offset:], ddsCubemap|ddsCubemapAllFaces)

	return header
}

// Faces of eight bit pixels, each of its own colour, of two levels, come out
// level by level, every face of a level together, as RGBA8.
func TestDecodeCubeUncompressed(t *testing.T) {
	file := ddsFile{
		width: 2, height: 2, levels: 2, flags: ddsRGB, bitCount: 32,
		masks: [4]uint32{0x00ff0000, 0x0000ff00, 0x000000ff, 0},
	}

	data := cubeHeader(file)

	for face := range cubeFaces {
		// Four pixels of the largest level, then one of the next, stored
		// blue, green, red, unused.
		for range 5 {
			data = append(data, byte(face), 0, byte(10+face), 0)
		}
	}

	cube, err := DecodeCube(data)
	if err != nil {
		t.Fatal(err)
	}

	if cube.Size != 2 || cube.Levels != 2 || cube.Format != RGBA8 || len(cube.Data) != cubeFaces*(4+1)*4 {
		t.Fatalf("cube = %d across, %d levels, %v, %d bytes", cube.Size, cube.Levels, cube.Format, len(cube.Data))
	}

	// The second face of the largest level starts after the four pixels of
	// the first, red, green, blue and an opaque alpha.
	if got := cube.Data[16:20]; !bytes.Equal(got, []byte{11, 0, 1, 255}) {
		t.Errorf("first pixel of the second face = %v, want red 11 and blue 1", got)
	}

	// The second level starts after the six faces of the first.
	if got := cube.Data[6*16+4 : 6*16+8]; !bytes.Equal(got, []byte{11, 0, 1, 255}) {
		t.Errorf("second face of the second level = %v", got)
	}
}

// Half floats and compressed blocks are kept as they are, faces reordered.
func TestDecodeCubeKeepsHalfFloatsAndBlocks(t *testing.T) {
	half := ddsFile{width: 1, height: 1, levels: 1, flags: ddsFourCC}
	data := cubeHeader(half)
	binary.LittleEndian.PutUint32(data[ddsFourCCOffset:], d3dA16B16G16R16F)

	for face := range cubeFaces {
		data = append(data, byte(face), 0, 0, 0, 0, 0, 0, 0x3c)
	}

	cube, err := DecodeCube(data)
	if err != nil {
		t.Fatal(err)
	}

	if cube.Format != RGBA16F || len(cube.Data) != cubeFaces*8 || cube.Data[8] != 1 {
		t.Errorf("half floats = %v, %v", cube.Format, cube.Data)
	}

	blocks := ddsFile{width: 4, height: 4, levels: 1, flags: ddsFourCC, fourCC: "DXT1"}
	data = cubeHeader(blocks)

	for face := range cubeFaces {
		data = append(data, bytes.Repeat([]byte{byte(face)}, 8)...)
	}

	cube, err = DecodeCube(data)
	if err != nil {
		t.Fatal(err)
	}

	if cube.Format != DXT1 || len(cube.Data) != cubeFaces*8 || cube.Data[40] != 5 {
		t.Errorf("blocks = %v, %v", cube.Format, cube.Data)
	}
}

func TestDecodeCubeRefuses(t *testing.T) {
	flat := ddsFile{width: 2, height: 2, levels: 1, flags: ddsRGB, bitCount: 32, masks: [4]uint32{0xff, 0xff00, 0xff0000, 0}}
	plain := append(flat.header(), make([]byte, 16)...)

	short := append(cubeHeader(flat), make([]byte, 16*5)...)

	oblong := ddsFile{width: 2, height: 4, levels: 1, flags: ddsRGB, bitCount: 32, masks: flat.masks}

	for name, data := range map[string][]byte{
		"a flat texture":       plain,
		"five faces of pixels": short,
		"oblong faces":         append(cubeHeader(oblong), make([]byte, 6*32)...),
	} {
		if _, err := DecodeCube(data); err == nil {
			t.Errorf("%s: decoded without an error", name)
		}
	}
}

// TestDecodeInstalledCubes reads the environment maps of the installations
// PDX_GAME_DIR names, one in each of the three ways the games store theirs.
func TestDecodeInstalledCubes(t *testing.T) {
	value := os.Getenv("PDX_GAME_DIR")
	if value == "" {
		t.Skip("set PDX_GAME_DIR to one or more game folders to run this test")
	}

	for _, game := range filepath.SplitList(value) {
		matches, _ := filepath.Glob(filepath.Join(game, "*", "gfx", "map", "environment", "*.dds"))
		direct, _ := filepath.Glob(filepath.Join(game, "gfx", "map", "environment", "*.dds"))

		for _, path := range append(direct, matches...) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			// The folder holds a flat texture or two as well, and Europa
			// Universalis 5 a file named .dds that is none.
			if _, err := readDDSHeader(data); err != nil || binary.LittleEndian.Uint32(data[ddsCaps2Offset:])&ddsCubemap == 0 {
				t.Logf("%s: no cube map, left out", filepath.Base(path))

				continue
			}

			cube, err := DecodeCube(data)
			if err != nil {
				t.Errorf("%s: %v", path, err)

				continue
			}

			t.Logf("%s: %d across, %d levels, %v", filepath.Base(path), cube.Size, cube.Levels, cube.Format)
		}
	}
}
