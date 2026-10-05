# pdx-asset-go

Go packages for the 3D assets of Paradox games: reading their mesh and texture
files, loading the entities of their `.asset` files from a game and its mods,
and drawing them with raylib.

```
go get github.com/kaiser-chris/pdx-asset-go@latest
```

Needs Go 1.26, a C compiler for raylib's cgo, and on Linux the headers of
OpenGL, X11, Wayland and xkbcommon. The definitions of the `.asset` files and
the way a game and its mods combine come from
[pdx-parser-go](https://github.com/kaiser-chris/pdx-parser-go).

## Packages

The packages are layered from the file formats to the GPU. Only `render`
needs raylib and an OpenGL context; everything before it is plain Go and is
tested without a GPU.

| Package | What it does | Depends on |
|---|---|---|
| [`mesh`](mesh) | Reads the binary `.mesh` files: shapes, the meshes of each, levels of detail, skins, skeletons and locators. | nothing |
| [`texture`](texture) | Decodes the textures the models use: DDS of every format the games ship (DXT1, DXT3, DXT5, BC7, uncompressed), Targa and PNG. | nothing |
| [`model`](model) | Turns meshes into geometry a graphics library draws: right handed coordinates, pieces of at most 65535 vertices. | `mesh`, `texture` |
| [`entity`](entity) | Loads an entity of the `.asset` files by name from a game and its mods, with the textures of each part. | `mesh`, `texture`, `model`, pdx-parser-go |
| [`render`](render) | Uploads models to the GPU and draws them, from a camera that circles them. | `model`, raylib |

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
mesh loads. The shipped files read without a single warning.

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
- Textures the engine finds somewhere other than next to their asset file:
  about half of Victoria 3's are, and how the engine finds them is not settled
  yet. Those parts are drawn with a neutral stand in and reported.
- The scale of a pdxmesh and an entity.

## License

MIT, see [LICENSE](LICENSE). The DDS, Targa and BC7 decoders started as those
of [pdx-flag-builder-go](https://github.com/kaiser-chris/pdx-flag-builder-go).
