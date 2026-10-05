package mesh_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/mesh"
)

// TestReadInstalledMeshes reads every mesh file of a real installation, its
// DLCs and layers included. Every one must read, hand out only meshes that
// can be drawn, and read without a warning: a warning on a shipped file is a
// gap in the reader, not in the game.
//
// It is skipped unless PDX_GAME_DIR points at a game folder.
func TestReadInstalledMeshes(t *testing.T) {
	root := os.Getenv("PDX_GAME_DIR")
	if root == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	var files, shapes, meshes, skinned int

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".mesh") {
			return nil //nolint:nilerr // an unreadable folder is not this test's problem
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // nor is an unreadable file
		}

		file, err := mesh.Read(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)

			return nil
		}

		files++

		for _, warning := range file.Warnings {
			t.Errorf("%s: %s", path, warning)
		}

		checkFile(t, file)

		for _, shape := range file.Shapes {
			shapes++
			meshes += len(shape.Meshes)

			for _, read := range shape.Meshes {
				if read.Skin != nil {
					skinned++
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("read %d files: %d shapes, %d meshes, %d of them skinned", files, shapes, meshes, skinned)

	if files == 0 {
		t.Skip("the folder holds no mesh files")
	}

	spotCheckMaleBody(t, root)
}

// spotCheckMaleBody compares the male body of the portraits against what was
// read from it by hand. It does nothing for a game without it.
func spotCheckMaleBody(t *testing.T, root string) {
	t.Helper()

	var data []byte

	for _, layer := range []string{"", "in_game"} {
		read, err := os.ReadFile(filepath.Join(root, layer, "gfx", "models", "portraits", "male_body", "male_body.mesh"))
		if err == nil {
			data = read

			break
		}
	}

	if data == nil {
		return
	}

	file, err := mesh.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	// Europa Universalis 5 has a male body of its own, built differently.
	if len(file.Shapes) == 0 || len(file.Shapes[0].Skeleton) == 0 || file.Shapes[0].Skeleton[0].Name != "body_root" {
		t.Logf("the male body is not the one of Victoria 3; not compared")

		return
	}

	if len(file.Shapes) != 1 || file.Shapes[0].Name != "male_bodyShape" || len(file.Shapes[0].Meshes) != 1 {
		t.Fatalf("male body = %+v", file.Shapes)
	}

	shape := file.Shapes[0]
	body := shape.Meshes[0]

	if body.Skin == nil || body.Skin.Influences != 4 || len(body.Normals) == 0 || len(body.Tangents) == 0 || len(body.UV(0)) == 0 {
		t.Errorf("male body mesh: %d vertices, skin %+v", body.Vertices(), body.Skin)
	}

	if len(shape.Skeleton) < 2 || shape.Skeleton[0].Name != "body_root" || shape.Skeleton[0].Parent != -1 {
		t.Errorf("male body skeleton = %d bones starting %+v", len(shape.Skeleton), shape.Skeleton[0])
	}

	// A figure standing upright, its feet at the ground, about a metre and a
	// half tall in the centimetres the portraits are modelled in.
	low, high := body.Bounds()
	if low[1] < -1 || low[1] > 5 || high[1] < 120 || high[1] > 200 {
		t.Errorf("male body stands from %v to %v", low, high)
	}

	t.Logf("male body: %d vertices, %d triangles, %d bones", body.Vertices(), body.Triangles(), len(shape.Skeleton))
}
