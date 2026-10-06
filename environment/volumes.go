package environment

import (
	"path"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/script"
)

// The post effect volumes.
//
// What the environment file sets for the post effect, and for the fog, the
// games override with post effect volumes: posteffect_volumes.txt, in the
// folder paths.settings names as post_effects. Each volume is a block of
// posteffect_values, named, which may inherit the values of another and set
// more: saturation_scale, levels_min, colorbalance and the like, under the
// keys of the environment file. Which apply depends on where the camera is,
// how high, by day or night, over which region.
//
// A model shown on its own is in none of those places; what applies to it is
// the volume that applies everywhere: the one named default, or in the games
// that have none, the one named standard, which their default inherits. Its
// fog is left out: how far fog begins and ends the volumes of each height of
// the camera over the map set, and a model shown on its own is at none.

// The folder of the post effect volumes, its key in paths.settings, and the
// file of the volumes in it.
const (
	postEffectsKey     = "post_effects"
	defaultPostEffects = "gfx/map/post_effects"
	volumesFile        = "posteffect_volumes.txt"
)

// globalVolumes are the names of the volume that applies everywhere, in the
// order they are looked for.
var globalVolumes = []string{"default", "standard"}

// volumeFields returns the fields of the volume that applies everywhere,
// those it inherits first, and its name; nothing without one.
func volumeFields(text string) ([]script.Field, string) {
	volumes := map[string]script.Node{}

	for _, field := range script.Parse(text).Fields {
		if field.Key != "posteffect_values" {
			continue
		}

		if named, ok := field.Value.Get("name"); ok {
			if name, ok := named.Str(); ok && name != "" {
				volumes[name] = field.Value
			}
		}
	}

	for _, name := range globalVolumes {
		if _, ok := volumes[name]; !ok {
			continue
		}

		var chain []script.Node

		seen := map[string]bool{}
		for current := name; current != "" && !seen[current]; {
			seen[current] = true

			volume, ok := volumes[current]
			if !ok {
				break
			}

			chain = append(chain, volume)

			current = ""
			if inherited, ok := volume.Get("inherit"); ok {
				current, _ = inherited.Str()
			}
		}

		var fields []script.Field

		for index := len(chain) - 1; index >= 0; index-- {
			for _, field := range chain[index].Fields {
				if field.Key != "name" && field.Key != "inherit" && !strings.HasPrefix(field.Key, "fog_") {
					fields = append(fields, field)
				}
			}
		}

		return fields, name
	}

	return nil, ""
}

// volumesPath is where the post effect volumes are, by paths.settings.
func volumesPath(paths *script.Document) string {
	folder := defaultPostEffects

	if paths != nil {
		if named, ok := paths.Get(postEffectsKey); ok {
			if text, ok := named.Str(); ok && text != "" {
				folder = text
			}
		}
	}

	return path.Join(folder, volumesFile)
}
