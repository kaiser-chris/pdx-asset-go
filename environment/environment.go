// Package environment reads the environment a game lights its world with:
// the sun, the environment map and the light around, as its environment file
// sets them.
//
// The engine hands the values of the file to the shaders through a constant
// buffer, JominiEnvironment, whose members are named after the keys of the
// file: ambient_pos_x is AmbientPosX, cubemap_intensity CubemapIntensity. A
// few are named otherwise, which Constants lists. What the file holds besides,
// such as the settings of post processing, the shaders of a model do not
// read.
//
// The file is the one paths.settings names as gfx_environment_file, in every
// one of the three games gfx/map/environment/environment.txt.
package environment

import (
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/kaiser-chris/pdx-parser-go/script"
)

// Source reads the files of a game by their path below its root.
type Source interface {
	ReadFile(path string) ([]byte, error)
}

// pathsFile is where the engine reads the paths of its files from, and
// pathsKey the key of the environment file in it.
const (
	pathsFile = "paths.settings"
	pathsKey  = "gfx_environment_file"
)

// defaultFile is the environment file of all three games, for a set of
// folders without paths.settings, such as a mod's.
const defaultFile = "gfx/map/environment/environment.txt"

// Environment is what an environment file sets.
type Environment struct {
	// Path is the file, below the game's root.
	Path string

	// Constants are the values of the file by the member of
	// JominiEnvironment each sets, as the file writes them: SunDiffuse is
	// the colour of the sun, ToSunDir the direction towards it, in the
	// game's coordinates.
	Constants map[string][]float32

	// Cubemap is the environment map, below the game's root, and
	// CubemapYRotation how far it is turned about the vertical, in degrees.
	Cubemap          string
	CubemapYRotation float32

	// Post is what the file sets for the post effect, and Shadow for the
	// shadows of the sun.
	Post   Post
	Shadow Shadow

	// Volume is the post effect volume read over the file, by
	// LoadFileOnMap, or empty for none.
	Volume string

	// Defines are the graphics defines the constants the shaders share are
	// set from; see defines.go.
	Defines Defines
}

// engineSun is the sun the engine lights by for a file that sets none of its
// own, as Victoria 3's environment_greyscale.txt sets none. The files do
// not hold it; it is estimated from how the game's model editor draws a
// building by that file: lit from the right and in front, low, with no light
// on what faces the sky.
var engineSun = map[string][]float32{
	"SunDiffuse":   {1, 1, 1},
	"SunIntensity": {1.5},
	"ToSunDir":     {1, 0.15, -0.6},
}

// Constants maps the keys of an environment file whose member of
// JominiEnvironment is not named after them.
var Constants = map[string]string{
	"sun_color":     "SunDiffuse",
	"sun_direction": "ToSunDir",
	"fog_begin":     "FogBegin2",
	"fog_end":       "FogEnd2",
}

// Load finds the environment file of a game and reads it. Without the file
// it returns the defaults, with the error saying why.
func Load(source Source) (*Environment, error) {
	return LoadFile(source, ConfiguredPath(source))
}

// ConfiguredPath is the environment file the game lights its world with, as
// paths.settings names it.
func ConfiguredPath(source Source) string {
	if paths := readPaths(source); paths != nil {
		if named, ok := paths.Get(pathsKey); ok {
			if text, ok := named.Str(); ok && text != "" {
				return text
			}
		}
	}

	return defaultFile
}

// LoadFile reads an environment file of a game, such as one of those its
// model editor offers, with the game's graphics defines, the way the model
// editor lights a model by it. Without the file it returns the defaults,
// with the error saying why.
func LoadFile(source Source, path string) (*Environment, error) {
	return loadFile(source, path, false)
}

// LoadFileOnMap reads an environment file as LoadFile does, with the post
// effect volume that applies everywhere on the map read over it; see
// volumes.go. The model editor does without: it draws a model by
// environment_greyscale.txt without colour, which the volume's saturation
// would put back.
func LoadFileOnMap(source Source, path string) (*Environment, error) {
	return loadFile(source, path, true)
}

func loadFile(source Source, path string, onMap bool) (*Environment, error) {
	data, err := source.ReadFile(path)
	if err != nil {
		return Default(), fmt.Errorf("the environment file %s: %w", path, err)
	}

	fields := script.Parse(string(data)).Fields

	var volume string
	if onMap {
		if text, err := source.ReadFile(volumesPath(readPaths(source))); err == nil {
			var overrides []script.Field

			overrides, volume = volumeFields(string(text))
			fields = append(fields, overrides...)
		}
	}

	environment := read(path, fields)
	environment.Volume = volume
	environment.Defines = loadDefines(source)

	return environment, nil
}

// readPaths reads paths.settings, or nothing without one.
func readPaths(source Source) *script.Document {
	data, err := source.ReadFile(pathsFile)
	if err != nil {
		return nil
	}

	return script.Parse(string(data))
}

// environmentKeys are keys only an environment file sets, which tell one
// apart from the other files of the folder environment files are kept in,
// such as the settings of day and night.
var environmentKeys = []string{"sun_intensity", "sun_color", "sun_direction", "cubemap", "cubemap_intensity"}

// IsEnvironment reports whether a file is an environment file.
func IsEnvironment(text string) bool {
	document := script.Parse(text)

	for _, key := range environmentKeys {
		if _, ok := document.Get(key); ok {
			return true
		}
	}

	return false
}

// Read reads an environment file. What it does not set it leaves out.
func Read(path, text string) *Environment {
	return read(path, script.Parse(text).Fields)
}

// read reads the fields of an environment file, the later of two with the
// same key winning.
func read(path string, fields []script.Field) *Environment {
	environment := &Environment{Path: path, Constants: map[string][]float32{}, Defines: newDefines(maps.Clone(defineDefaults))}

	// The suns given by their azimuth and elevation, by scenario.
	suns := map[string][2]float32{}

	for _, field := range fields {
		switch field.Key {
		case "cubemap":
			if text, ok := field.Value.Str(); ok {
				environment.Cubemap = text
			}

			continue
		case "cubemap_y_rotation":
			if number, ok := field.Value.Num(); ok {
				environment.CubemapYRotation = float32(number)
			}

			continue
		}

		if isShadowKey(field.Key) {
			continue
		}

		if scenario, angle, ok := sunAngle(field.Key); ok {
			if number, ok := field.Value.Num(); ok {
				angles := suns[scenario]
				angles[angle] = float32(number)
				suns[scenario] = angles
			}

			continue
		}

		values, ok := numbers(field.Value)
		if !ok {
			continue
		}

		name, ok := Constants[field.Key]
		if !ok {
			name = constantName(field.Key)
		}

		environment.Constants[name] = values
	}

	for scenario, angles := range suns {
		environment.Constants[sunConstant(scenario)] = sunDirection(angles[0], angles[1])
	}

	// The sun of the engine, for a file that sets none.
	for name, values := range engineSun {
		if _, ok := environment.Constants[name]; !ok {
			environment.Constants[name] = append([]float32(nil), values...)
		}
	}

	environment.Post = readPost(fields, environment.Constants)
	environment.Shadow = readShadow(fields)

	return environment
}

// Crusader Kings 3 gives the suns of its scenarios, terrain_sunny,
// map_objects_overcast and the like, by their azimuth and elevation:
// terrain_sunny_sun_azimuth and terrain_sunny_sun_elevation set
// ToTerrainSunnySunDir.
const (
	sunAzimuth   = "_sun_azimuth"
	sunElevation = "_sun_elevation"
)

// sunAngle splits the key of a sun's angle into its scenario and which angle
// it is: 0 for the azimuth, 1 for the elevation.
func sunAngle(key string) (scenario string, angle int, ok bool) {
	if scenario, ok := strings.CutSuffix(key, sunAzimuth); ok && scenario != "" {
		return scenario, 0, true
	}

	if scenario, ok := strings.CutSuffix(key, sunElevation); ok && scenario != "" {
		return scenario, 1, true
	}

	return "", 0, false
}

// sunConstant is the member of JominiEnvironment the sun of a scenario sets.
func sunConstant(scenario string) string {
	return "To" + constantName(scenario) + "SunDir"
}

// sunDirection is the direction towards a sun of an azimuth and elevation,
// as the shaders that read them describe both: an azimuth of 0 is north,
// 0.25 west, 0.5 south and 0.75 east, and an elevation of 0 the horizon, 0.5
// 45 degrees up and 1 the zenith. North is +z, east +x, up +y.
func sunDirection(azimuth, elevation float32) []float32 {
	around := float64(azimuth) * 2 * math.Pi
	up := float64(elevation) * math.Pi / 2

	return []float32{
		float32(-math.Sin(around) * math.Cos(up)),
		float32(math.Sin(up)),
		float32(math.Cos(around) * math.Cos(up)),
	}
}

// numbers reads a number, a list of numbers, or a colour written as
// rgb { } or hsv { }, which comes back as red, green and blue.
func numbers(value script.Node) ([]float32, bool) {
	if number, ok := value.Num(); ok {
		return []float32{float32(number)}, true
	}

	tag := ""
	if value.Kind == script.KindTagged && value.Value != nil {
		tag = strings.ToLower(value.Tag)
		value = *value.Value
	}

	if tag == "hex" {
		return hexColor(value)
	}

	list, ok := value.Numbers()
	if !ok || len(list) == 0 {
		return nil, false
	}

	values := make([]float32, len(list))
	for index, number := range list {
		values[index] = float32(number)
	}

	switch tag {
	case "":
	case "hsv":
		if len(values) != 3 {
			return nil, false
		}

		red, green, blue := hsvToRGB(values[0], values[1], values[2])
		values = []float32{red, green, blue}
	case "rgb":
		// The games write rgb colours from 0 to 255 or from 0 to 1.
		if slicesMax(values) > 1 {
			for index := range values {
				values[index] /= 255
			}
		}
	default:
		return nil, false
	}

	return values, true
}

// hexColor reads a colour written as hex { rrggbb }, or with its alpha as
// hex { rrggbbaa }, each channel from 0 to 1.
func hexColor(value script.Node) ([]float32, bool) {
	text := value.Text
	if len(value.Items) == 1 {
		text = value.Items[0].Text
	}

	text = strings.TrimPrefix(strings.TrimSpace(text), "0x")
	if len(text) != 6 && len(text) != 8 {
		return nil, false
	}

	values := make([]float32, 0, len(text)/2)

	for index := 0; index < len(text); index += 2 {
		channel, err := strconv.ParseUint(text[index:index+2], 16, 8)
		if err != nil {
			return nil, false
		}

		values = append(values, float32(channel)/255)
	}

	return values, true
}

func slicesMax(values []float32) float32 {
	largest := values[0]
	for _, value := range values[1:] {
		largest = max(largest, value)
	}

	return largest
}

// hsvToRGB turns a colour of hue, saturation and value, each from 0 to 1,
// into red, green and blue.
func hsvToRGB(hue, saturation, value float32) (red, green, blue float32) {
	hue = float32(math.Mod(float64(hue), 1))
	if hue < 0 {
		hue++
	}

	sector := hue * 6
	whole := float32(math.Floor(float64(sector)))
	fraction := sector - whole

	low := value * (1 - saturation)
	falling := value * (1 - saturation*fraction)
	rising := value * (1 - saturation*(1-fraction))

	switch int(whole) % 6 {
	case 0:
		return value, rising, low
	case 1:
		return falling, value, low
	case 2:
		return low, value, rising
	case 3:
		return low, falling, value
	case 4:
		return rising, low, value
	}

	return value, low, falling
}

// constantName turns a key of the file into the name of its member of
// JominiEnvironment: ambient_pos_x into AmbientPosX.
func constantName(key string) string {
	var name strings.Builder

	for _, word := range strings.Split(key, "_") {
		if word == "" {
			continue
		}

		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		name.WriteString(string(runes))
	}

	return name.String()
}
