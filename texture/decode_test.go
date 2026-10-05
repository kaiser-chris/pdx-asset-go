package texture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestDecodeChoosesTheReaderByExtension(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	picture.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	picture.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 128})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}

	targa := append(targaHeader(tgaTrueColor, 1, 1, 24, tgaTopToBottomBit), 0x00, 0x00, 0xff)
	dds := fakeDDS("DXT1", ddsFourCC, 4, 4, 1, 8)

	for name, test := range map[string]struct {
		data   []byte
		format Format
		first  [4]byte
	}{
		"picture.PNG":  {encoded.Bytes(), RGBA8, [4]byte{255, 0, 0, 255}},
		"pattern.tga":  {targa, RGBA8, [4]byte{255, 0, 0, 255}},
		"diffuse.dds":  {dds, DXT1, [4]byte{1, 2, 1, 2}},
		"in/a/dir.Dds": {dds, DXT1, [4]byte{1, 2, 1, 2}},
	} {
		image, err := Decode(name, test.data)
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}

		if image.Format != test.format || image.Levels != 1 || [4]byte(image.Data) != test.first {
			t.Errorf("%s: %v with %d levels starting %v, want %v starting %v", name, image.Format, image.Levels, image.Data[:4], test.format, test.first)
		}
	}

	// A partly transparent pixel keeps its colour rather than being darkened
	// by its alpha.
	decoded, _ := Decode("picture.png", encoded.Bytes())
	if [4]byte(decoded.Data[4:8]) != [4]byte{0, 255, 0, 128} {
		t.Errorf("the transparent pixel is %v, want its colour kept", decoded.Data[4:8])
	}
}

func TestDecodeRefusesWhatItCannotRead(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty.dds":     {},
		"picture.jpg":   []byte("some bytes"),
		"no_extension":  []byte("some bytes"),
		"broken.png":    []byte("\x89PNG\r\n\x1a\nbroken"),
		"broken.tga":    {1, 2, 3},
		"broken.dds":    []byte("DDS "),
		"rubbish.dds":   bytes.Repeat([]byte{0xff}, 200),
		"truncated.png": {0x89, 'P', 'N', 'G'},
	} {
		if image, err := Decode(name, data); err == nil {
			t.Errorf("%s: read as %+v", name, image)
		} else if !strings.Contains(err.Error(), name) {
			t.Errorf("%s: the error %q does not name the file", name, err)
		}
	}
}

// A header that claims far more pixels than the file holds is refused before
// anything is allocated for them.
func TestDecodeDoesNotTrustHeaders(t *testing.T) {
	cases := map[string][]byte{
		"targa":                append(targaHeader(tgaTrueColor, 65535, 65535, 32, 0), 1, 2, 3, 4),
		"run length targa":     append(targaHeader(tgaRunLengthColor, 65535, 65535, 32, 0), 0xff, 1, 2, 3, 4),
		"uncompressed DDS":     append(ddsFile{width: MaxSize, height: MaxSize, flags: ddsRGB, bitCount: 32}.header(), 1, 2, 3, 4),
		"block compressed DDS": append(ddsFile{width: MaxSize, height: MaxSize, flags: ddsFourCC, fourCC: "DXT5"}.header(), 1, 2, 3, 4),
		"BC7 DDS":              append(ddsFile{width: MaxSize, height: MaxSize, flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiBC7Unorm}.header(), 1, 2, 3, 4),
	}

	for name, data := range cases {
		done := make(chan error, 1)

		go func() {
			_, err := Decode(name+".dds", data)
			if strings.Contains(name, "targa") {
				_, err = Decode(name+".tga", data)
			}

			done <- err
		}()

		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s: a header claiming more than the file holds was read", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: took longer than five seconds", name)
		}
	}
}

// A run length Targa that is exactly as long as its pixels need is still read.
func TestDecodeTGARunLengthAtItsShortest(t *testing.T) {
	file := append(targaHeader(tgaRunLengthColor, 128, 1, 24, tgaTopToBottomBit), 0xff, 0x00, 0x00, 0xff)

	pixels, err := DecodeTGA(file)
	if err != nil {
		t.Fatal(err)
	}

	if pixels.Width != 128 || pixels.Data[127*4] != 0xff {
		t.Errorf("pixels = %dx%d, last %v", pixels.Width, pixels.Height, pixels.Data[127*4:])
	}
}

// FuzzDecode runs every reader over inputs the fuzzer makes up. Run it with
//
//	go test ./texture -run '^$' -fuzz FuzzDecode -fuzztime 60s
func FuzzDecode(f *testing.F) {
	var indices [16]uint32

	for _, seed := range [][]byte{
		fakeDDS("DXT5", ddsFourCC, 8, 8, 2, 16),
		fakeDDS("DXT1", ddsFourCC|ddsAlphaPixels, 4, 4, 1, 8),
		append(ddsFile{width: 1, height: 1, levels: 1, flags: ddsRGB | ddsAlphaPixels, bitCount: 32, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}}.header(), 1, 2, 3, 4),
		append(ddsFile{width: 4, height: 4, levels: 1, flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiBC7Unorm}.header(), mode6Block(indices)...),
		append(ddsFile{width: 1, height: 1, levels: 1, flags: ddsFourCC, fourCC: "DX10", dxgi: dxgiB8G8R8A8Unorm}.header(), 1, 2, 3, 4),
		append(targaHeader(tgaTrueColor, 1, 1, 24, 0), 1, 2, 3),
		append(targaHeader(tgaRunLengthColor, 2, 1, 32, 0), 0x81, 1, 2, 3, 4),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, name := range []string{"f.dds", "f.tga", "f.png"} {
			image, err := Decode(name, data)
			if err != nil {
				continue
			}

			checkImage(t, name, image)
		}
	})
}

// checkImage asserts that a decoded image is as large as it says it is.
func checkImage(t *testing.T, name string, image *Image) {
	t.Helper()

	if image.Width <= 0 || image.Height <= 0 || image.Width > MaxSize || image.Height > MaxSize || image.Levels < 1 {
		t.Fatalf("%s: decoded to %dx%d with %d levels", name, image.Width, image.Height, image.Levels)
	}

	want := image.Width * image.Height * 4
	if image.Format.Compressed() {
		want = levelsSize(image.Width, image.Height, image.Levels, image.Format)
	} else if image.Levels != 1 {
		t.Fatalf("%s: decoded pixels with %d levels, want 1", name, image.Levels)
	}

	if len(image.Data) != want {
		t.Fatalf("%s: %v %dx%d with %d levels holds %d bytes, want %d", name, image.Format, image.Width, image.Height, image.Levels, len(image.Data), want)
	}
}
