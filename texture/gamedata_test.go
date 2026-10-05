package texture

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDecodeInstalledTextures decodes every texture below the gfx/models
// folders of a real installation, which is where the textures of every 3D
// model are, in both Victoria 3 and the layers of Europa Universalis 5.
//
// It is skipped unless PDX_GAME_DIR points at a game folder. Set PDX_DUMP_DIR
// as well to write what was decoded to pixels out as PNG files, which is the
// only way to actually see whether a decoder is right.
func TestDecodeInstalledTextures(t *testing.T) {
	root := os.Getenv("PDX_GAME_DIR")
	if root == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	dump := os.Getenv("PDX_DUMP_DIR")
	if dump != "" {
		if err := os.MkdirAll(dump, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	counts := map[Format]int{}

	visit := func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //nolint:nilerr // an unreadable folder is not this test's problem
		}

		extension := strings.ToLower(filepath.Ext(path))
		if extension != ".tga" && extension != ".dds" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // nor is an unreadable file
		}

		if shippedBroken(path, data) {
			return nil
		}

		decoded, err := Decode(path, data)
		if err != nil {
			t.Errorf("%v", err)

			return nil
		}

		checkImage(t, path, decoded)
		counts[decoded.Format]++

		if dump != "" && decoded.Format == RGBA8 {
			writePNG(t, filepath.Join(dump, filepath.Base(path)+".png"), decoded)
		}

		return nil
	}

	folders := modelFolders(root)
	if len(folders) == 0 {
		t.Skip("the folder holds no gfx/models folder")
	}

	for _, folder := range folders {
		if err := filepath.WalkDir(folder, visit); err != nil {
			t.Fatalf("walk %s: %v", folder, err)
		}
	}

	t.Logf("decoded %v", counts)
}

// shippedBroken reports the files the games ship that are not textures at
// all, in spite of their name: Europa Universalis 5 has a couple of DDS files
// shorter than a DDS header.
func shippedBroken(path string, data []byte) bool {
	return strings.EqualFold(filepath.Ext(path), ".dds") && len(data) < ddsHeaderSize
}

// modelFolders finds the gfx/models folders of a game, in the game itself and
// in each of its layers and DLCs.
func modelFolders(root string) []string {
	var found []string

	bases := []string{root}

	for _, parent := range []string{root, filepath.Join(root, "dlc")} {
		entries, _ := os.ReadDir(parent)
		for _, entry := range entries {
			if entry.IsDir() {
				bases = append(bases, filepath.Join(parent, entry.Name()))

				// The DLCs of a game with layers have layers of their own.
				inner, _ := os.ReadDir(filepath.Join(parent, entry.Name()))
				for _, layer := range inner {
					if layer.IsDir() {
						bases = append(bases, filepath.Join(parent, entry.Name(), layer.Name()))
					}
				}
			}
		}
	}

	for _, base := range bases {
		folder := filepath.Join(base, "gfx", "models")
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			found = append(found, folder)
		}
	}

	return found
}

func writePNG(t *testing.T, path string, decoded *Image) {
	t.Helper()

	picture := image.NewNRGBA(image.Rect(0, 0, decoded.Width, decoded.Height))

	for y := range decoded.Height {
		for x := range decoded.Width {
			offset := (y*decoded.Width + x) * 4
			picture.SetNRGBA(x, y, color.NRGBA{
				R: decoded.Data[offset], G: decoded.Data[offset+1], B: decoded.Data[offset+2], A: decoded.Data[offset+3],
			})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()

	if err := png.Encode(file, picture); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}
