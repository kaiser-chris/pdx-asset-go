package texture

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// ddsFile lays out the header of a DDS file. The pixels or blocks are for the
// caller to append.
type ddsFile struct {
	width, height, levels int
	flags                 uint32
	fourCC                string
	bitCount              int
	masks                 [4]uint32
	dxgi                  uint32
}

func (f ddsFile) header() []byte {
	header := make([]byte, ddsHeaderSize)
	copy(header[0:4], "DDS ")
	binary.LittleEndian.PutUint32(header[ddsSizeOffset:], ddsHeaderLength)
	binary.LittleEndian.PutUint32(header[ddsHeightOffset:], uint32(f.height))
	binary.LittleEndian.PutUint32(header[ddsWidthOffset:], uint32(f.width))
	binary.LittleEndian.PutUint32(header[ddsMipCountOffset:], uint32(f.levels))
	binary.LittleEndian.PutUint32(header[76:], 32)
	binary.LittleEndian.PutUint32(header[ddsPixelFlagsOffset:], f.flags)
	copy(header[ddsFourCCOffset:], f.fourCC)
	binary.LittleEndian.PutUint32(header[ddsBitCountOffset:], uint32(f.bitCount))

	for index, mask := range f.masks {
		binary.LittleEndian.PutUint32(header[ddsMasksOffset+index*4:], mask)
	}

	if f.dxgi != 0 {
		extended := make([]byte, ddsExtendedSize)
		binary.LittleEndian.PutUint32(extended, f.dxgi)
		binary.LittleEndian.PutUint32(extended[4:], 3) // a two dimensional texture
		header = append(header, extended...)
	}

	return header
}

// fakeDDS lays out a block compressed DDS file: the header, then the blocks
// of every mipmap level the way a real file stores them, whole blocks each.
func fakeDDS(fourCC string, flags uint32, width, height, levels, blockSize int) []byte {
	data := ddsFile{width: width, height: height, levels: levels, flags: flags, fourCC: fourCC}.header()

	return appendBlocks(data, width, height, levels, blockSize)
}

func appendBlocks(data []byte, width, height, levels, blockSize int) []byte {
	for level := 0; level < levels; level++ {
		blocks := (max(width>>level, 1) + 3) / 4 * ((max(height>>level, 1) + 3) / 4)
		for index := range blocks * blockSize {
			// Each level marked by its number, to tell the first from the rest.
			data = append(data, byte(level+1+index%2))
		}
	}

	return data
}

// A header claiming millions of levels is believed only as far as halving
// the image goes, and read in no time, rather than measured level by level.
// The fuzzer found this one: 808 million levels, read quadratically.
func TestDecodeDDSDoesNotTrustItsLevelCount(t *testing.T) {
	data := fakeDDS("DXT1", ddsFourCC, 64, 32, 7, 8)
	binary.LittleEndian.PutUint32(data[ddsMipCountOffset:], 0x30303030)

	done := make(chan *Image, 1)

	go func() {
		image, _ := DecodeDDS(data)
		done <- image
	}()

	select {
	case image := <-done:
		if image == nil || image.Levels != 7 {
			t.Errorf("image = %+v, want the seven levels of a 64x32 image", image)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading took longer than five seconds")
	}
}

// The texture that crashed the Store version of the flag builder: 768 by 512,
// DXT5, with every mipmap level down to one pixel.
func TestDecodeDDSMeasuresEveryLevel(t *testing.T) {
	data := fakeDDS("DXT5", ddsFourCC, 768, 512, 10, 16)

	image, err := DecodeDDS(data)
	if err != nil {
		t.Fatalf("DecodeDDS: %v", err)
	}

	if image.Width != 768 || image.Height != 512 || image.Format != DXT5 {
		t.Errorf("image = %dx%d %v, want 768x512 DXT5", image.Width, image.Height, image.Format)
	}

	if image.Levels != 10 {
		t.Errorf("levels = %d, want the ten the file holds", image.Levels)
	}

	// Every level, each rounded up to whole blocks.
	want := 0
	for level := range 10 {
		across, down := max(768>>level, 1), max(512>>level, 1)
		want += (across + 3) / 4 * ((down + 3) / 4) * 16
	}

	if len(image.Data) != want {
		t.Errorf("blocks = %d bytes, want %d", len(image.Data), want)
	}

	// And the levels together come to more than raylib's rule of thumb of a
	// third over the largest, which is what read past the end of the buffer.
	if largest := 192 * 128 * 16; want <= largest*4/3 {
		t.Errorf("the levels come to %d bytes, want more than the %d raylib would have allowed",
			want, largest*4/3)
	}
}

// A file that holds fewer levels than its header claims is read for the ones
// it does hold, rather than being refused or read past its end.
func TestDecodeDDSReadsTheLevelsAFileHolds(t *testing.T) {
	data := fakeDDS("DXT5", ddsFourCC, 64, 64, 3, 16)

	// The header says four, the blocks are three levels long.
	binary.LittleEndian.PutUint32(data[ddsMipCountOffset:], 4)

	image, err := DecodeDDS(data)
	if err != nil {
		t.Fatal(err)
	}

	if image.Levels != 3 {
		t.Errorf("levels = %d, want the three the file holds", image.Levels)
	}
}

func TestDecodeDDSBlockFormats(t *testing.T) {
	tests := []struct {
		name      string
		file      ddsFile
		blockSize int
		want      Format
	}{
		{"DXT1", ddsFile{fourCC: "DXT1", flags: ddsFourCC}, 8, DXT1},
		{"DXT1 with alpha", ddsFile{fourCC: "DXT1", flags: ddsFourCC | ddsAlphaPixels}, 8, DXT1Alpha},
		{"DXT3", ddsFile{fourCC: "DXT3", flags: ddsFourCC}, 16, DXT3},
		{"DXT5", ddsFile{fourCC: "DXT5", flags: ddsFourCC}, 16, DXT5},
		{"BC1 behind DX10", ddsFile{fourCC: "DX10", flags: ddsFourCC, dxgi: dxgiBC1Unorm}, 8, DXT1Alpha},
		{"BC2 behind DX10", ddsFile{fourCC: "DX10", flags: ddsFourCC, dxgi: dxgiBC2UnormSRGB}, 16, DXT3},
		{"BC3 behind DX10", ddsFile{fourCC: "DX10", flags: ddsFourCC, dxgi: dxgiBC3Unorm}, 16, DXT5},
	}

	for _, test := range tests {
		test.file.width, test.file.height, test.file.levels = 64, 32, 1

		image, err := DecodeDDS(appendBlocks(test.file.header(), 64, 32, 1, test.blockSize))
		if err != nil || image.Format != test.want || len(image.Data) != 16*8*test.blockSize || !image.Format.Compressed() {
			t.Errorf("%s: %+v, %v", test.name, image, err)
		}
	}
}

// An edge that is not a multiple of four still takes whole blocks.
func TestDecodeDDSRoundsToWholeBlocks(t *testing.T) {
	image, err := DecodeDDS(fakeDDS("DXT5", ddsFourCC, 30, 10, 1, 16))
	if err != nil {
		t.Fatal(err)
	}

	if want := 8 * 3 * 16; len(image.Data) != want {
		t.Errorf("blocks = %d bytes, want %d for 8 by 3 blocks", len(image.Data), want)
	}
}

func TestDecodeDDSRefusesAFileCutShort(t *testing.T) {
	data := fakeDDS("DXT5", ddsFourCC, 64, 64, 1, 16)

	if _, err := DecodeDDS(data[:len(data)-1]); err == nil {
		t.Error("a file one byte short was read")
	}
}

// The portraits' diffuse and normal maps are 32 bit pixels in the order
// blue, green, red, alpha, which the masks say.
func TestDecodeDDSUncompressedBGRA(t *testing.T) {
	file := ddsFile{
		width: 2, height: 2, levels: 1, flags: ddsRGB | ddsAlphaPixels, bitCount: 32,
		masks: [4]uint32{0x00ff0000, 0x0000ff00, 0x000000ff, 0xff000000},
	}

	data := append(file.header(),
		0x30, 0x20, 0x10, 0x40, // stored blue, green, red, alpha
		0xff, 0x00, 0x00, 0xff,
		0x00, 0xff, 0x00, 0x80,
		0x00, 0x00, 0xff, 0x00,
	)

	image, err := DecodeDDS(data)
	if err != nil {
		t.Fatal(err)
	}

	want := []byte{
		0x10, 0x20, 0x30, 0x40,
		0x00, 0x00, 0xff, 0xff,
		0x00, 0xff, 0x00, 0x80,
		0xff, 0x00, 0x00, 0x00,
	}

	if image.Format != RGBA8 || image.Levels != 1 || !bytes.Equal(image.Data, want) {
		t.Errorf("image = %v %d levels %v, want %v", image.Format, image.Levels, image.Data, want)
	}
}

// Every layout of uncompressed pixels comes out the same way, whatever the
// order and width of its channels.
func TestDecodeDDSUncompressedLayouts(t *testing.T) {
	tests := []struct {
		name   string
		file   ddsFile
		pixel  []byte
		expect [4]byte
	}{
		{
			"24 bit BGR",
			ddsFile{flags: ddsRGB, bitCount: 24, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0}},
			[]byte{0x03, 0x02, 0x01}, [4]byte{0x01, 0x02, 0x03, 0xff},
		},
		{
			"32 bit RGBA",
			ddsFile{flags: ddsRGB | ddsAlphaPixels, bitCount: 32, masks: [4]uint32{0xff, 0xff00, 0xff0000, 0xff000000}},
			[]byte{0x01, 0x02, 0x03, 0x04}, [4]byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			"alpha mask the flags do not use",
			ddsFile{flags: ddsRGB, bitCount: 32, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}},
			[]byte{0x03, 0x02, 0x01, 0x00}, [4]byte{0x01, 0x02, 0x03, 0xff},
		},
		{
			"16 bit 565",
			ddsFile{flags: ddsRGB, bitCount: 16, masks: [4]uint32{0xf800, 0x07e0, 0x001f, 0}},
			[]byte{0xff, 0xff}, [4]byte{0xff, 0xff, 0xff, 0xff},
		},
		{
			"16 bit 4444",
			ddsFile{flags: ddsRGB | ddsAlphaPixels, bitCount: 16, masks: [4]uint32{0x0f00, 0x00f0, 0x000f, 0xf000}},
			[]byte{0x80, 0x0f}, [4]byte{0xff, 0x88, 0x00, 0x00},
		},
		{
			"8 bit grey",
			ddsFile{flags: ddsLuminance, bitCount: 8, masks: [4]uint32{0xff, 0, 0, 0}},
			[]byte{0x7f}, [4]byte{0x7f, 0x7f, 0x7f, 0xff},
		},
		{
			"8 bit alpha",
			ddsFile{flags: ddsAlphaOnly | ddsAlphaPixels, bitCount: 8, masks: [4]uint32{0, 0, 0, 0xff}},
			[]byte{0x7f}, [4]byte{0x00, 0x00, 0x00, 0x7f},
		},
		{
			"B8G8R8A8 behind DX10",
			ddsFile{flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiB8G8R8A8UnormSRGB},
			[]byte{0x03, 0x02, 0x01, 0x04}, [4]byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			"B8G8R8X8 behind DX10",
			ddsFile{flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiB8G8R8X8Unorm},
			[]byte{0x03, 0x02, 0x01, 0x04}, [4]byte{0x01, 0x02, 0x03, 0xff},
		},
		{
			"R8G8B8A8 behind DX10",
			ddsFile{flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiR8G8B8A8Unorm},
			[]byte{0x01, 0x02, 0x03, 0x04}, [4]byte{0x01, 0x02, 0x03, 0x04},
		},
	}

	for _, test := range tests {
		test.file.width, test.file.height, test.file.levels = 1, 1, 1

		image, err := DecodeDDS(append(test.file.header(), test.pixel...))
		if err != nil {
			t.Errorf("%s: %v", test.name, err)

			continue
		}

		if [4]byte(image.Data) != test.expect {
			t.Errorf("%s: pixel = %v, want %v", test.name, image.Data, test.expect)
		}
	}
}

// Rows of an uncompressed file are read in order, the first being the top.
func TestDecodeDDSUncompressedRows(t *testing.T) {
	file := ddsFile{width: 3, height: 2, levels: 2, flags: ddsRGB, bitCount: 24, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0}}

	data := file.header()
	for index := range 6 {
		data = append(data, 0, 0, byte(index))
	}

	// The second level, which is ignored, follows.
	data = append(data, 9, 9, 9)

	image, err := DecodeDDS(data)
	if err != nil {
		t.Fatal(err)
	}

	for index := range 6 {
		if image.Data[index*4] != byte(index) {
			t.Fatalf("pixel %d has red %d, want %d", index, image.Data[index*4], index)
		}
	}
}

func TestDecodeDDSRefusesRubbish(t *testing.T) {
	valid := ddsFile{width: 4, height: 4, levels: 1, flags: ddsRGB, bitCount: 32, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0}}

	withSize := func(size uint32) []byte {
		data := append(valid.header(), make([]byte, 64)...)
		binary.LittleEndian.PutUint32(data[ddsSizeOffset:], size)

		return data
	}

	huge := valid
	huge.width, huge.height = 100000, 100000

	big := valid
	big.width, big.height = MaxSize, MaxSize

	cases := map[string][]byte{
		"empty":                    {},
		"not a DDS file":           []byte("PNG and then some bytes that are long enough to be read as a header, and some more ....................................................."),
		"a header of another size": withSize(100),
		"no size":                  ddsFile{flags: ddsRGB, bitCount: 32}.header(),
		"a size beyond reason":     huge.header(),
		"pixels cut short":         append(valid.header(), make([]byte, 63)...),
		"a header claiming a lot":  big.header(),
		"an unknown compression":   append(ddsFile{width: 4, height: 4, flags: ddsFourCC, fourCC: "ATI2"}.header(), make([]byte, 16)...),
		"an unknown DX10 format":   append(ddsFile{width: 4, height: 4, flags: ddsFourCC, fourCC: "DX10", dxgi: 2}.header(), make([]byte, 256)...),
		"a DX10 header cut short":  ddsFile{width: 4, height: 4, flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiBC7Unorm}.header()[:ddsHeaderSize+4],
		"pixels of 12 bits":        append(ddsFile{width: 4, height: 4, flags: ddsRGB, bitCount: 12}.header(), make([]byte, 64)...),
		"no format at all":         append(ddsFile{width: 4, height: 4}.header(), make([]byte, 64)...),
	}

	for name, data := range cases {
		if image, err := DecodeDDS(data); err == nil {
			t.Errorf("%s: read as %+v", name, image)
		}
	}
}

func TestIsBC7(t *testing.T) {
	file := ddsFile{width: 4, height: 4, levels: 1, flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiBC7Unorm}.header()

	if !IsBC7(file) {
		t.Error("a BC7 file was not recognised")
	}

	if IsBC7(fakeDDS("DXT5", ddsFourCC, 4, 4, 1, 16)) {
		t.Error("a DXT5 file was taken for BC7")
	}

	if IsBC7([]byte("not a dds file at all")) {
		t.Error("rubbish was taken for BC7")
	}
}

func TestDecodeDDSBC7(t *testing.T) {
	data := ddsFile{width: 4, height: 4, levels: 1, flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiBC7UnormSRGB}.header()

	var indices [16]uint32

	image, err := DecodeDDS(append(data, mode6Block(indices)...))
	if err != nil {
		t.Fatal(err)
	}

	if image.Format != RGBA8 || image.Width != 4 || len(image.Data) != 4*4*4 {
		t.Errorf("image = %+v", image)
	}
}

// A picture of four half floats a pixel, the way Crusader Kings 3 stores the
// table it tonemaps by (tony_mc_mapface_2d.dds): a legacy header naming
// D3DFMT_A16B16G16R16F, 113, as its four character code, or a DX10 header
// naming R16G16B16A16_FLOAT. The largest level comes out as it is stored.
func TestDecodeDDSHalfFloats(t *testing.T) {
	var fourCC [4]byte
	binary.LittleEndian.PutUint32(fourCC[:], d3dA16B16G16R16F)

	for name, file := range map[string]ddsFile{
		"legacy code 113": {flags: ddsFourCC, fourCC: string(fourCC[:])},
		"behind DX10":     {flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiR16G16B16A16Float},
	} {
		file.width, file.height, file.levels = 3, 2, 2

		// Six pixels of the largest level, each of its own, then a smaller
		// level that is left out.
		pixels := make([]byte, 3*2*8)
		for index := range pixels {
			pixels[index] = byte(index)
		}

		data := append(file.header(), pixels...)
		data = append(data, make([]byte, 8)...)

		decoded, err := DecodeDDS(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if decoded.Format != RGBA16F || decoded.Width != 3 || decoded.Height != 2 || decoded.Levels != 1 || !bytes.Equal(decoded.Data, pixels) {
			t.Errorf("%s: decoded %v %dx%d with %d levels, %d bytes; want the largest level of half floats as stored",
				name, decoded.Format, decoded.Width, decoded.Height, decoded.Levels, len(decoded.Data))
		}

		if _, err := DecodeDDS(data[:len(data)-len(pixels)]); err == nil {
			t.Errorf("%s: a file cut short was read", name)
		}
	}
}
