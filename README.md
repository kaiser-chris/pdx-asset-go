# pdx-asset-go

Go packages for the 3D assets of Paradox games: reading their mesh and texture
files, loading the entities of their `.asset` files from a game and its mods,
and drawing them with raylib.

```
go get github.com/kaiser-chris/pdx-asset-go@latest
```

Needs Go 1.26, a C and C++ compiler for cgo, and on Linux the headers of
OpenGL, X11, Wayland and xkbcommon. The definitions of the `.asset` files and
the way a game and its mods combine come from
[pdx-parser-go](https://github.com/kaiser-chris/pdx-parser-go).

## Packages

The packages are layered from the file formats to the GPU. Only `render`
needs raylib and an OpenGL context; everything before it is tested without a
GPU, and everything but `shader` is plain Go.

| Package | What it does | Depends on |
|---|---|---|
| [`mesh`](mesh) | Reads the binary `.mesh` files: shapes, the meshes of each, levels of detail, skins, skeletons and locators. | nothing |
| [`texture`](texture) | Decodes the textures the models use: DDS of every format the games ship (DXT1, DXT3, DXT5, BC7, uncompressed), Targa and PNG. | nothing |
| [`model`](model) | Turns meshes into geometry a graphics library draws: right handed coordinates, pieces of at most 65535 vertices. | `mesh`, `texture` |
| [`entity`](entity) | Loads an entity of the `.asset` files by name from a game and its mods, with the textures of each part. | `mesh`, `texture`, `model`, pdx-parser-go |
| [`shader`](shader) | Reads the games' shader files and builds the effects the assets name, compiled from their HLSL to GLSL. | glslang, SPIRV-Tools and SPIRV-Cross, vendored |
| [`render`](render) | Uploads models to the GPU and draws them with the games' effects, from a camera that circles them. | `model`, `shader`, raylib |

## Loading and drawing an entity

```go
set := folders.Open([]folders.Source{{Name: "game", Path: gameFolder}})
loader := entity.NewLoader(set, asset.Load(set))

built, problems, err := loader.Load("male_body_entity") // plain Go, can run off the drawing thread

renderer, _ := render.NewRenderer() // needs the OpenGL context from here on
uploaded, _ := renderer.Upload(built)

viewer := renderer.NewViewer(512, 512)
viewer.Frame(uploaded.Min, uploaded.Max)
viewer.Rotate(math.Pi/4, 0.1)
viewer.Draw(uploaded) // into viewer.Target()
```

## Coordinates

The games compose their scenes the way Direct3D does, in left handed
coordinates, and wind their triangles anticlockwise around the outward normal
in right handed terms. `model.Convert` mirrors every model across x and winds
its triangles the other way round, which leaves it looking as it does in the
game with its front faces in front, so back face culling works. The tangent
sign is flipped with it, so that normal maps light the way they do in the
game.

## Where textures are found

A texture a mesh settings block names is looked for next to the asset file
that names it. Many are not there: a building names the decal it shares with
a dozen others by its bare file name, and the decal is in a folder of its own.
The engine finds those through a lookup of the textures below `gfx/models` by
file name, which it builds at start and reports in its debug log:

```
[pdxassetutil.cpp:304]: ThreadedInitTextureLookup: gfx/models
[pdxassetutil.cpp:313]: ThreadedInitTextureLookup found 5610 files
```

The `entity` loader builds the same lookup. Its contents follow from the
counts the games log, which it reproduces to the file:

- the `.dds` and `.tga` files below `gfx/models`, but no `.png`;
- of the game, its DLCs and its mods together, each path once, so that a
  mod's file replacing one of the game's counts once;
- in a game split into layers, as Europa Universalis 5 is, the layers mounted
  one on top of the other: `loading_screen`, then `main_menu`, then
  `in_game`. An asset file sees the textures of its own layer and of the
  layers below it.

Should two files of one name meet in the lookup, which the shipped files never
do, the one of the latest folder in load order is taken and the choice is
reported, since what the engine does then is not known.

## Shaders

A part is drawn with the effect its mesh settings name, such as `standard` of
`gfx/FX/pdxmesh.shader`, built from the game's shader files and those of its
engine, the `clausewitz` and `jomini` folders next to the game folder.

The shader files hold HLSL in a format of Paradox's own: structs with
semantics, constant buffers, samplers, entry points and effects. The `shader`
package puts an effect together the way the engine does, with the engine's
own prelude, and compiles it with glslang to SPIR-V, legalizes that with
SPIRV-Tools and writes it out with SPIRV-Cross as GLSL 3.30, which raylib
draws with. The three libraries of the Khronos Group are vendored in
`internal/glslcross` and compiled by cgo; the first build takes a few minutes.

The engine's GLSL prelude, `defines_glsl4.fxh`, is still shipped, but the code
no longer compiles as GLSL through it: not one effect of the three games does.
Through HLSL every effect the shipped assets use compiles, 149 of Victoria 3,
71 of Europa Universalis 5 and 107 of Crusader Kings 3.

What the engine gives an effect while the game runs, the viewer gives in the
most basic form there is: a texture the asset does not give reads white, a
colour is white, a matrix the identity, any other number zero, and the
camera's constants are the camera's. A part whose effect cannot be built is
drawn with the renderer's own shader, and the model says why.

glslang has declared its HLSL front end deprecated, to be removed at its next
major version, so the vendored libraries cannot be updated without checking
that HLSL still reads.

## Robustness

As in pdx-parser-go, the readers do not trust their input:

- A count in a mesh or texture header is checked against the bytes that back
  it before anything is allocated, so a broken file cannot ask for gigabytes,
  and a mipmap count is believed only as far as halving the image goes.
- A mesh file whose structure is broken is refused; inside a well formed file,
  data that does not add up, such as an attribute of the wrong length or a
  skin that does not fit its mesh, is left out as narrowly as possible and
  listed in its warnings.
- An entity whose texture is missing is drawn with a neutral stand in, and
  the problem is reported.

The test suites include hand written hostile input, every way of cutting a
file short, fuzz targets for both binary readers, and GPU tests that check
what is actually drawn: the front of a model the way round the game shows it,
its back culled, compressed textures in their colours, meshes too large for
sixteen bit indices drawn whole.

The readers have been run over every `.mesh` file and every model texture of
Victoria 3 and Europa Universalis 5, and every entity of both that draws a
mesh loads. The shipped files read without a single warning, and every
texture the entities of Victoria 3, Europa Universalis 5 and Crusader Kings 3
name is found.

## Development

```
make test        # everything that needs neither a GPU nor a game
make uitest      # also the GPU tests, which open a hidden window and need a display
make gametest PDX_GAME_DIR="C:/Steam/steamapps/common/Victoria 3/game"
make fuzz        # a minute of fuzzing for each binary reader
```

With `PDX_GAME_DIR` set, the tests also read and draw a real installation;
the shipped files are expected to read without warnings, which makes them the
quickest way to find out what a game update changed.
`PDX_DUMP_DIR` additionally writes decoded textures and rendered views of the
male body out as PNG files, which is how to see that they look right.

## Not done yet

- Skinning and animation: models are drawn in the pose their mesh files store.
- Blend shapes, and the attributes of an entity that drive them.
- The scale of a pdxmesh and an entity.

## License

MIT, see [LICENSE](LICENSE). The DDS, Targa and BC7 decoders started as those
of [pdx-flag-builder-go](https://github.com/kaiser-chris/pdx-flag-builder-go).

The vendored libraries in `internal/glslcross` keep their own licences, next to
their sources: [glslang](https://github.com/KhronosGroup/glslang),
[SPIRV-Tools](https://github.com/KhronosGroup/SPIRV-Tools),
[SPIRV-Headers](https://github.com/KhronosGroup/SPIRV-Headers) and
[SPIRV-Cross](https://github.com/KhronosGroup/SPIRV-Cross).
