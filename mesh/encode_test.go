package mesh_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
	"github.com/kaiser-chris/pdx-asset-go/mesh/meshtest"
)

// A file decoded and encoded again comes out as it went in, every kind of
// property and an empty one among them.
func TestEncodeWritesWhatDecodeReads(t *testing.T) {
	data := meshtest.New().
		Object(1, "a").Strings("names", "first", "", "third").
		Object(2, "b").Ints("n", 1, -2).
		Object(3, "c").
		Object(2, "d").Floats("f").Floats("g", 0.5, -1e9).
		Object(1, "e").Bytes()

	root, err := mesh.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := mesh.Encode(root)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encoded, data) {
		t.Errorf("encoded %q, want %q", encoded, data)
	}
}

// What the format cannot hold is refused rather than written broken.
func TestEncodeRefusesWhatAFileCannotHold(t *testing.T) {
	for name, root := range map[string]*mesh.Object{
		"a long property name": {Children: []*mesh.Object{{Name: "a", Properties: []mesh.Property{{Name: strings.Repeat("x", 256), Kind: mesh.Ints}}}}},
		"a zero in a name":     {Children: []*mesh.Object{{Name: "a\x00b"}}},
		"an unknown kind":      {Children: []*mesh.Object{{Name: "a", Properties: []mesh.Property{{Name: "p", Kind: 'q'}}}}},
	} {
		if _, err := mesh.Encode(root); err == nil {
			t.Errorf("%s was written", name)
		}
	}
}

// TestEncodeInstalledMeshes decodes and encodes the portrait meshes of a real
// installation, which must come out byte for byte as the game ships them.
//
// It is skipped unless PDX_GAME_DIR points at a game folder.
func TestEncodeInstalledMeshes(t *testing.T) {
	root := os.Getenv("PDX_GAME_DIR")
	if root == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	var files int

	err := filepath.WalkDir(filepath.Join(root, "gfx", "models", "portraits"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".mesh") {
			return nil //nolint:nilerr // an unreadable folder is not this test's problem
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // nor is an unreadable file
		}

		decoded, err := mesh.Decode(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)

			return nil
		}

		encoded, err := mesh.Encode(decoded)
		if err != nil {
			t.Errorf("%s: %v", path, err)
		} else if !bytes.Equal(encoded, data) {
			t.Errorf("%s: encoded %d bytes differ from the %d shipped", path, len(encoded), len(data))
		}

		files++

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if files == 0 {
		t.Skip("the folder holds no portrait meshes")
	}

	t.Logf("encoded %d files", files)
}
