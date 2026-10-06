package texture

import "testing"

// The texture of a missing one is squares of magenta and black, eight
// across, opaque.
func TestCheckerboard(t *testing.T) {
	board := Checkerboard()

	if board.Width != 64 || board.Height != 64 || board.Format != RGBA8 || len(board.Data) != 64*64*4 {
		t.Fatalf("checkerboard = %dx%d %v of %d bytes", board.Width, board.Height, board.Format, len(board.Data))
	}

	pixel := func(x, y int) [4]byte { return [4]byte(board.Data[(y*64+x)*4:]) }

	magenta, black := [4]byte{255, 0, 255, 255}, [4]byte{0, 0, 0, 255}

	for _, test := range []struct {
		x, y int
		want [4]byte
	}{
		{0, 0, magenta}, {7, 7, magenta}, {8, 0, black}, {0, 8, black}, {8, 8, magenta}, {63, 63, magenta}, {56, 0, black},
	} {
		if got := pixel(test.x, test.y); got != test.want {
			t.Errorf("pixel %d,%d = %v, want %v", test.x, test.y, got, test.want)
		}
	}
}
