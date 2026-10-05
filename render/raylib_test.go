package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// raylibSource reads a file of the raylib sources raylib-go ships, or skips
// the test when the module is not at hand.
func raylibSource(t *testing.T, name string) string {
	t.Helper()

	output, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/gen2brain/raylib-go/raylib").Output()
	if err != nil {
		t.Skipf("the raylib-go module is not at hand: %v", err)
	}

	source, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(output)), name))
	if err != nil {
		t.Fatal(err)
	}

	return string(source)
}

// The pixel formats handed to raylib have to be the numbers raylib's C enum
// gives them, which raylib-go's constants are not. raylib.h is read from the
// raylib-go module, so an update that renumbers them fails here.
func TestPixelFormatsMatchRaylib(t *testing.T) {
	header := raylibSource(t, "raylib.h")

	// Read the enum the way the C compiler numbers it.
	enum := regexp.MustCompile(`(?s)typedef enum \{\s*(PIXELFORMAT_UNCOMPRESSED_GRAYSCALE = 1.*?)\} PixelFormat;`).FindStringSubmatch(header)
	if enum == nil {
		t.Fatal("the PixelFormat enum was not found in raylib.h")
	}

	values := map[string]int{}
	next := 0

	for _, line := range strings.Split(enum[1], "\n") {
		name := regexp.MustCompile(`^\s*(PIXELFORMAT_\w+)\s*(?:=\s*(\d+))?`).FindStringSubmatch(line)
		if name == nil {
			continue
		}

		if name[2] != "" {
			next, _ = strconv.Atoi(name[2])
		}

		values[name[1]] = next
		next++
	}

	for format, name := range map[texture.Format]string{
		texture.RGBA8:     "PIXELFORMAT_UNCOMPRESSED_R8G8B8A8",
		texture.DXT1:      "PIXELFORMAT_COMPRESSED_DXT1_RGB",
		texture.DXT1Alpha: "PIXELFORMAT_COMPRESSED_DXT1_RGBA",
		texture.DXT3:      "PIXELFORMAT_COMPRESSED_DXT3_RGBA",
		texture.DXT5:      "PIXELFORMAT_COMPRESSED_DXT5_RGBA",
	} {
		if got, want := int(pixelFormats[format]), values[name]; got != want {
			t.Errorf("%s is handed to raylib as %d, raylib.h says %d", name, got, want)
		}
	}
}

// Emptying a material before it is freed walks every map raylib gives it, so
// the count has to be raylib's.
func TestMaterialMapsMatchRaylib(t *testing.T) {
	found := regexp.MustCompile(`#define MAX_MATERIAL_MAPS\s+(\d+)`).FindStringSubmatch(raylibSource(t, "rmodels.c"))
	if found == nil {
		t.Fatal("MAX_MATERIAL_MAPS was not found in rmodels.c")
	}

	if want, _ := strconv.Atoi(found[1]); materialMaps != want {
		t.Errorf("materialMaps = %d, raylib says %d", materialMaps, want)
	}
}

// The shaders are built into the package.
func TestShadersAreEmbedded(t *testing.T) {
	for _, name := range []string{"shaders/standard.vs", "shaders/standard.fs"} {
		source, err := shaders.ReadFile(name)
		if err != nil || !strings.HasPrefix(string(source), "#version 330") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
