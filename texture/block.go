package texture

// The block compressions keep four by four pixels in one block, which a
// graphics card works out as it draws them. The few palettes of a game that are
// stored that way are read back a pixel at a time here, which is all a palette
// of sixteen by a few pixels is ever asked for.

// Colour returns the red, green, blue and alpha of one pixel of an image.
//
// A pixel of an image whose format is a block compression is worked out from
// the block it is in, as the graphics card would; a pixel of an image the
// package has no reader for is not there to be read, which is what the second
// result says.
func (i *Image) Colour(x, y int) ([4]byte, bool) {
	if i == nil || x < 0 || y < 0 || x >= i.Width || y >= i.Height {
		return [4]byte{}, false
	}

	if i.Format == RGBA8 {
		at := (y*i.Width + x) * 4
		if at+4 > len(i.Data) {
			return [4]byte{}, false
		}

		return [4]byte{i.Data[at], i.Data[at+1], i.Data[at+2], i.Data[at+3]}, true
	}

	if !i.Format.Compressed() {
		return [4]byte{}, false
	}

	// A DXT1 block is its two colours and the indices between them; the
	// blocks that hold alpha as well are twice that.
	size := 8
	if i.Format == DXT3 || i.Format == DXT5 {
		size = 16
	}

	// The largest level comes first, and a palette has only that one.
	wide := (i.Width + 3) / 4
	block := ((y/4)*wide + x/4) * size
	if block+size > len(i.Data) {
		return [4]byte{}, false
	}

	return blockColour(i.Format, i.Data[block:block+size], (y%4)*4+x%4)
}

// blockColour returns one pixel of a block, by its place in it, counting from
// the left of the top row.
//
// A DXT1 block is its two colours and the indices into them; the blocks that
// hold alpha as well begin with it, a DXT3 block with four bits a pixel and a
// DXT5 block with two ends and eight values between them.
func blockColour(format Format, block []byte, at int) ([4]byte, bool) {
	switch format {
	case DXT1, DXT1Alpha:
		colours, transparent := colourPalette(block[0:4])
		index := (little(block[4:8]) >> (2 * at)) & 3
		chosen := colours[index]

		// The three colour form of a block with alpha spells its fourth
		// colour as nothing at all.
		if transparent && format == DXT1Alpha && index == 3 {
			chosen[3] = 0
		}

		return chosen, true

	case DXT3:
		colours, _ := colourPalette(block[8:12])
		chosen := colours[(little(block[12:16])>>(2*at))&3]

		// Four bits of alpha for every pixel, in the order the pixels are in,
		// the low half of a byte first.
		chosen[3] = block[at/2] >> (4 * (at % 2)) & 0xf
		chosen[3] *= 17 // four bits to eight

		return chosen, true

	case DXT5:
		colours, _ := colourPalette(block[8:12])
		chosen := colours[(little(block[12:16])>>(2*at))&3]
		chosen[3] = alphaPalette(block[0], block[1])[(alphaIndices(block[2:8])>>(3*at))&7]

		return chosen, true
	}

	return [4]byte{}, false
}

// colourPalette is the four colours of a block, from the two it holds as five
// and six bit numbers. The fourth is the transparent black of the three colour
// form, which a block is in when its two colours are the wrong way round.
func colourPalette(endpoints []byte) ([4][4]byte, bool) {
	first, second := uint16(endpoints[0])|uint16(endpoints[1])<<8, uint16(endpoints[2])|uint16(endpoints[3])<<8

	var colours [4][4]byte

	colours[0] = rgb565(first)
	colours[1] = rgb565(second)

	if first > second {
		for axis := range 3 {
			colours[2][axis] = byte((2*int(colours[0][axis]) + int(colours[1][axis])) / 3)
			colours[3][axis] = byte((int(colours[0][axis]) + 2*int(colours[1][axis])) / 3)
		}

		colours[2][3], colours[3][3] = 255, 255

		return colours, false
	}

	for axis := range 3 {
		colours[2][axis] = byte((int(colours[0][axis]) + int(colours[1][axis])) / 2)
		colours[3][axis] = 0
	}

	// The fourth colour of the three colour form is nothing at all, which
	// only a block with alpha spells that way.
	colours[2][3] = 255

	return colours, true
}

// alphaPalette is the eight values of alpha a DXT5 block holds.
func alphaPalette(first, second byte) [8]byte {
	var values [8]byte

	values[0], values[1] = first, second

	if first > second {
		for index := range 6 {
			values[2+index] = byte((int(first)*(6-index) + int(second)*(index+1)) / 7)
		}
	} else {
		for index := range 4 {
			values[2+index] = byte((int(first)*(4-index) + int(second)*(index+1)) / 5)
		}

		values[6], values[7] = 0, 255
	}

	return values
}

// rgb565 is a colour held as five bits of red, six of green and five of blue,
// as eight bits of each.
func rgb565(value uint16) [4]byte {
	return [4]byte{
		byte((value >> 11 & 0x1f) * 255 / 31),
		byte((value >> 5 & 0x3f) * 255 / 63),
		byte((value & 0x1f) * 255 / 31),
		255,
	}
}

// little reads four bytes as the number they are, the first of them lowest.
func little(bytes []byte) uint32 {
	return uint32(bytes[0]) | uint32(bytes[1])<<8 | uint32(bytes[2])<<16 | uint32(bytes[3])<<24
}

// alphaIndices reads the six bytes of a DXT5 block that hold eight three bit
// values as the number they are, the first of them lowest.
func alphaIndices(bytes []byte) uint64 {
	var value uint64

	for index, one := range bytes {
		value |= uint64(one) << (8 * index)
	}

	return value
}
