package texture

// Checkerboard is a texture that says a texture is missing, the way game
// engines show one: squares of magenta and black, eight across, which
// stand out against anything a model is textured with.
func Checkerboard() *Image {
	const (
		size  = 64
		cells = 8
	)

	image := &Image{Width: size, Height: size, Format: RGBA8, Levels: 1, Data: make([]byte, size*size*4)}

	for y := range size {
		for x := range size {
			pixel := image.Data[(y*size+x)*4:]
			pixel[3] = 255

			if (x*cells/size+y*cells/size)%2 == 0 {
				pixel[0], pixel[2] = 255, 255
			}
		}
	}

	return image
}
