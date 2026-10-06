# pdx-asset-go

Go packages for the 3D assets of Paradox games: reading their mesh and texture
files, loading the entities of their `.asset` files from a game and its mods,
and drawing them with raylib.

```
go get github.com/kaiser-chris/pdx-asset-go@latest
```

Needs Go 1.26, a C compiler for cgo, and on Linux the headers of
OpenGL, X11, Wayland and xkbcommon. The definitions of the `.asset` files and
the way a game and its mods combine come from
[pdx-parser-go](https://github.com/kaiser-chris/pdx-parser-go).

## Packages

The packages are layered from the file formats to the GPU. Only `render`
needs raylib and an OpenGL context; everything before it is plain Go, tested
without a GPU.

| Package | What it does | Depends on |
|---|---|---|
| [`mesh`](mesh) | Reads the binary `.mesh` files: shapes, the meshes of each, levels of detail, skins, skeletons and locators. | nothing |
| [`anim`](anim) | Reads the binary `.anim` files: the joints of a skeletal animation, its rate and every frame of its samples. | `mesh` |
| [`texture`](texture) | Decodes the textures the models use: DDS of every format the games ship (DXT1, DXT3, DXT5, BC7, uncompressed), Targa and PNG. | nothing |
| [`model`](model) | Turns meshes into geometry a graphics library draws: right handed coordinates, pieces of at most 65535 vertices. | `mesh`, `texture` |
| [`entity`](entity) | Loads an entity of the `.asset` files by name from a game and its mods, or from a plain folder, with the textures of each part. | `mesh`, `anim`, `texture`, `model`, pdx-parser-go |
| [`render`](render) | Uploads models to the GPU and draws them with a shader that approximates the games' look, from a camera that circles them. | `model`, raylib |

## Loading and drawing an entity

```go
set := folders.Open(append(entity.EngineFolders(gameFolder), folders.Source{Name: "game", Path: gameFolder}))
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

## Drawing

The games draw every part with an effect of their shader files, which the
mesh settings name, lit by the environment of the map. The renderer does not
use those: it draws every part with a shader of its own, an approximation of
the games' look. It reads the material the way the games' shaders read it
(the colour, the normal map with x in green and y in alpha, roughness and
metalness in the properties map) and lights it with a few lights and the
light of a sky.

What the games' effects do beyond that it tells from the name of the effect a
part's settings give, which the three games name alike: the palette colour is
blended into skin (`portrait_skin`), leaves and hair are cut out by their alpha
(`tree`, `hair`, `alpha_to_coverage`), decals are laid over the rest by theirs
(`decal_local`, or a part in a subpass of decals, drawn after the solid
geometry), atlases are read by the second set of texture coordinates
(`standard_atlas`), and parts named two sided are seen from both sides.

The leaves of the games' trees are grey in their diffuse maps: the games
colour them as they draw them, from the colour of the map where a tree stands
and a tint each tree effect takes in its slot 3. The loader reads that tint,
and the renderer overlays the leaves with its middle colour, as Crusader Kings
3 and Europa Universalis 5 do. A part of a tree effect without a tint has what
is grey of it overlaid with a green of leaves, and keeps what has a colour of
its own, such as bark.

## Attachments

An entity attaches others to points of its own, and the loader draws them
with it, in one model, each where it hangs. The name an entity attaches is
global: the entity can be defined in any asset file below `gfx`. The point it
hangs from is looked for the way the shipped files of the three games use
them:

- a locator of the entity, or of an entity it clones, with a position, a
  rotation in degrees and a scale, as the hubs of Victoria 3 place a town;
- a locator of its mesh file, by the matrix the file stores, which holds a
  scale as well, as Europa Universalis 5 puts a horse in front of a wagon;
  one that hangs from a bone is placed relative to the bone, as the oars of
  a galley hang from its deck;
- a bone of its mesh, in the pose the mesh is stored in, as the cavalry of
  Victoria 3 put a rider on the saddle and a weapon in the rider's hand.

An entity attaches what the entity it clones attaches, but an attach block of
a name replaces the one of that name it clones, as the dragoon of Victoria 3
swaps the sabre of the hussar it clones for a rifle. A group the engine picks
from at random is drawn with its first choice. An entity can draw no mesh and
be nothing but its attachments.

Every part of the model says which entity it is of, and the model lists what
is attached to what. An entity attached that is not defined, or whose point
is not found, is left out, listed as missing and reported; one that draws no
mesh, such as smoke from a chimney, is drawn as nothing without a word.

## Animations

A mesh names the animations it can play, each a binary `.anim` file, and may
import a whole set of them made for another mesh's skeleton, which is how
Crusader Kings 3 plays the animations of its male body on its female one. The
[`anim`](anim) package reads those files, and the loader lists with each model
what its entities, and the entities they attach, can play.

Every one of the 3651 files the three games ship is written the same way:
a rate, a count of samples and of joints, a joint per bone of the skeleton
saying which of its translation, rotation and scale the samples change and the
pose it starts in, then those samples frame by frame. The rate is written as
the samples over the length rather than the rate the animation was made at, so
an animation of 150 samples at 15.100671 frames a second is ten seconds of a
clip made at 15; read that way, 2897 of the shipped files give a whole rate of
15, 24, 25, 30 or 60, against 38 read the other way round.

Joints drive the bones of their name, whatever their case and without the
namespace the rig was exported under, so `zeppelin_01_rig:root` drives the
bone `root`. Matched that way, all but eighty odd of the 7687 pairs of mesh
and animation the games ship line up bone for bone, in order; the rest share
animations between skeletons that differ.

Listing an animation reads the head of its file rather than the whole of it:
one body of Crusader Kings 3 can play 765 animations, which together are near
five hundred megabytes, while their heads are 64 KB at most and hold the rate,
the frames and the joints a list needs. The samples of the one played are read
when it is, and the `skin` package moves the geometry with them, the step the
games do in their vertex shaders.

## Accessories

A portrait accessory, such as the belts, coats and sashes Victoria 3 and
Crusader Kings 3 hang on a character, is coloured by a *variation*: a named
block in `gfx/portraits/accessory_variations` that holds the ways the accessory
may be patterned and the palettes it may be coloured in. The entity's own game
data names the variation and the mask that says where each pattern goes.

The [`pattern`](pattern) package reads those files: every `pattern_textures`,
`pattern_layout` and `variation` of a game and its mods, by name. An entity's
`game_data` is carried through as the raw node the parser read, so the loader
reads the accessory out of it itself, and every part of the entity carries a
[`model.Accessory`](model/accessory.go): the mask, and every alternative the
variation offers, each with its own textures and placements read and decoded.

The games pick one pattern and one palette of a variation at random for every
portrait they draw, and within a palette one of four shades of each channel and
one of its rows. A viewer draws the first of each, and offers the patterns and
palettes themselves to choose between; `render.Model.ChooseAccessory` switches
one without uploading the model again. The palette's colours are read out of
its texture rather than sampled from it, a part having more textures to draw
with than a material has places to put them, and a palette is read in whichever
of the three shapes its game wrote it in: sixteen pixels wide with four shades
of a channel side by side, four wide with one pixel for each, or four high with
one row for each. A palette stored as blocks — a few percent of them — is read
a pixel at a time through the block it is in.

What a pattern *is* was read off the textures rather than out of any
documentation, since the shaders that lay them are compiled into the games:
the pattern texture is a mask whose channels say which of its regions covers a
pixel, not a colour to multiply the palette by, so the plain silk that most of
the shipped patterns are — one region covering everything, written in the red
channel — draws the colour of the palette rather than red. The colours
themselves come from the palette, four columns for each channel of the mask,
one column of four shades per channel.

A `pattern_textures` block names a normal map and a properties map beside its
colour mask, and every one of the shipped patterns does: they are the *surface*
of the accessory, not a detail over it, since the pattern is the material the
part is made of. A normal map off a pattern is not flat — every block of one of
their DXT5 files differs from the next — and the properties map says what the
material is, metal or cloth. The renderer therefore draws the surface of the
pattern in place of the mesh's own where the pattern covers a pixel, and the
mesh's own maps where it does not, one channel of the mask after another.

Drawing all of that takes thirteen textures per part, where a material has
twelve maps of which only a few are free, so the renderer binds them to texture
units of its own: the mask to the unit raylib keeps for a material's roughness
map, which a part drawn with a pattern has no tint for, and the rest above the
four raylib binds a material's own maps to. Sixteen units are what an OpenGL
3.3 context is sure to have.

## Files outside a game

An `entity.Folder` is a plain folder of asset files, such as a modder keeps
outside the game, which a loader reads like the folders of a game. Three
options of a loader suit such a folder: `ByName` looks for a mesh or texture
that is not where its asset file says by its file name next to it, and for an
entity attached that is not defined in the asset files next to it,
`MissingTexture` stands in for a texture of colour that is not there, such as
`texture.Checkerboard`, and `EmptyWithoutMesh` loads an entity whose mesh is
missing as nothing rather than failing.

## Robustness

As in pdx-parser-go, the readers do not trust their input:

- A count in a mesh or texture header is checked against the bytes that back
  it before anything is allocated, so a broken file cannot ask for gigabytes,
  and a mipmap count is believed only as far as halving the image goes.
- A mesh file whose structure is broken is refused; inside a well formed file,
  data that does not add up, such as an attribute of the wrong length or a
  skin that does not fit its mesh, is left out as narrowly as possible and
  listed in its warnings.
- An entity whose texture is missing is drawn with a neutral stand in, or the
  loader's `MissingTexture`, and the problem is reported.
- An entity attached that cannot be drawn is left out and reported; an
  entity attached to itself, through others, is attached once.

The test suites include hand written hostile input, every way of cutting a
file short, fuzz targets for both binary readers, and GPU tests that check
what is actually drawn: the front of a model the way round the game shows it,
its back culled, compressed textures in their colours, meshes too large for
sixteen bit indices drawn whole.

The readers have been run over every `.mesh` file and every model texture of
Victoria 3 and Europa Universalis 5, and every entity of both that draws a
mesh loads. The shipped files read without a single warning, and every
texture the entities of Victoria 3, Europa Universalis 5 and Crusader Kings 3
name is found. Of the attachments of the three, over 1400, all are drawn but
six, which the shipped files attach to locators that do not exist or to an
entity that is not defined.

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

- Blend shapes, and the attributes of an entity that drive them.
- The scale of a pdxmesh and an entity.

## License

MIT, see [LICENSE](LICENSE). The DDS, Targa and BC7 decoders started as those
of [pdx-flag-builder-go](https://github.com/kaiser-chris/pdx-flag-builder-go).
