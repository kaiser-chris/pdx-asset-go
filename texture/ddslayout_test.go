package texture

// The layout of a DDS header, which the tests write files by. The package
// reads them with dds-go and needs none of it.

// Offsets into a DDS file. The header is a fixed layout, so the few fields that
// matter can be read straight out of it.
const (
	ddsMagicSize        = 4
	ddsHeaderSize       = 128 // magic plus the 124 byte header
	ddsExtendedSize     = 20  // the DX10 header that may follow
	ddsSizeOffset       = 4
	ddsHeightOffset     = 12
	ddsWidthOffset      = 16
	ddsPixelFlagsOffset = 80
	ddsFourCCOffset     = 84
	ddsBitCountOffset   = 88
	ddsMasksOffset      = 92
	ddsDXGIFormatOffs   = 128
	ddsCaps2Offset      = 112
	ddsMiscFlagOffset   = 136
)

// The size the header says it is, which every well formed file writes.
const ddsHeaderLength = 124

// Pixel format flags.
const (
	// ddsAlphaPixels is set when the pixels carry alpha: for DXT1 it means
	// the blocks use their one bit of alpha, for uncompressed pixels that the
	// alpha mask is in use.

	// ddsAlphaOnly marks a file holding nothing but alpha.
	ddsAlphaOnly = 0x2

	// ddsFourCC says the format is named by its four character code.
	ddsFourCC = 0x4

	// ddsRGB and ddsLuminance say the pixels are stored uncompressed, as
	// colour or as grey.
	ddsRGB       = 0x40
	ddsLuminance = 0x20000
)

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

// The formats of four half floats a channel: the D3D one, written in the
// four character code of a file without a DX10 header, and the DXGI one.
const (
	d3dA16B16G16R16F      = 113
	dxgiR16G16B16A16Float = 10
)

// The flags that say a DDS file holds a cubemap: in the caps of its header,
// and in the misc flags of a DX10 header.
const (
	ddsCaps2Cubemap = 0x200
	dx10TextureCube = 0x4
)

// bc7BlockSize is how many bytes a BC7 block takes.
const bc7BlockSize = 16

// levelsSize is how many bytes the given number of mipmap levels of a block
// compression take: each level half the size of the one before it, down to a
// pixel, every edge rounded up to whole blocks of four.
func levelsSize(width, height, levels int, format Format) int {
	size := 0

	for level := range levels {
		across, down := max(width>>level, 1), max(height>>level, 1)
		size += (across + 3) / 4 * ((down + 3) / 4) * format.BlockSize()
	}

	return size
}
