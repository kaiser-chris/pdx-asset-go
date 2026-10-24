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
| [`texture`](texture) | Decodes the textures the models use: DDS of every format, through [dds-go](https://github.com/kaiser-chris/dds-go), Targa and PNG. | dds-go |
| [`model`](model) | Turns meshes into geometry a graphics library draws: right handed coordinates, pieces of at most 65535 vertices. | `mesh`, `texture` |
| [`entity`](entity) | Loads an entity of the `.asset` files by name from a game and its mods, or from a plain folder, with the textures and blend shapes of each part. | `mesh`, `anim`, `texture`, `model`, pdx-parser-go |
| [`state`](state) | Plays the states of an entity, as its `.asset` definition writes them: the animation each plays, picked by chance among states of one name, and the next state each goes on to. | pdx-parser-go |
| [`state`](state) | Plays the states of an entity, as its `.asset` definition writes them: the animation each plays, picked by chance among states of one name, and the next state each goes on to. | pdx-parser-go |
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
viewer.Pan(0, viewer.UnitsPerPixel()*100) // the model a hundred pixels down
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
part's settings give, which the three games name alike: a palette colour is
blended into skin, eyes and hair, the colour of each its own (`portrait_skin`,
`portrait_eye`, `portrait_hair`; `Look.PaletteColor`, `EyeColor` and
`HairColor`), where the diffuse map's alpha says for skin and eyes and the blue
of the normal map for hair, whose alpha cuts its strands out; leaves and hair are cut out by their alpha
(`tree`, `hair`, `alpha_to_coverage`), and the glass and lace of portrait
attachments drawn with alpha to coverage
(`portrait_attachment_alpha_to_coverage`) are solid where their alpha is whole
and laid over what is behind them by it elsewhere, as the games' coverage of
several samples to a pixel leaves them, after everything solid of every model
a viewer draws, decals are laid over the rest by theirs
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

## Blend shapes and additive animations

A mesh may name *blend shapes*, shapes it can be morphed towards, such as a
bigger nose or a fatter body. Each is a mesh file of its own that holds the
mesh morphed fully into the shape: the same vertices in the same order, moved.
The loader reads them with the mesh, matches their meshes to the mesh's by
their place among the most detailed shapes, since their shapes are named after
the blend shape rather than the mesh, and `model.Convert` turns each into how
far it moves every vertex, converted and cut into pieces with the rest.
Europa Universalis 5 names what some shapes change, `data = "normal|tangent"`:
those shade a face its additive animations move, and their positions are
those of another mesh, so only what the asset names is taken.
Europa Universalis 5 names what some shapes change, `data = "normal|tangent"`:
those shade a face its additive animations move, and their positions are
those of another mesh, so only what the asset names is taken.

What morphs a mesh is its entity's *attributes*, each of which drives one of
the blend shapes:

```
attribute = { name = "head_bs_nose_size_max" blend_shape = "male_head_bs_nose_size_max" }
```

The games set them from a character's genes, which
[pdx-parser-go](https://github.com/kaiser-chris/pdx-parser-go)'s `genes`
package reads. `render.Model.Morph` takes the values of the attributes by name
and blends the shapes in on the CPU, the way a skinned part is posed: a
portrait changes only when its genes do, so there is nothing to gain from
doing it on the GPU every frame. A skinned part is morphed first and posed
from its morphed vertices.

```go
uploaded.Morph(map[string]float64{"head_bs_nose_size_max": 0.5})
```

The male head of Victoria 3 loads with every one of its blend shapes, each
driven by one of its attributes.

The other attributes of a portrait's head play *additive animations*: short
clips, seven frames in every one Victoria 3 ships, one for each feature, such
as how far forward the chin is. The loader reads them whole with the mesh,
and `Morph` plays each at the place along it its attribute picks, which the
`skin` package lays over whatever pose the skeleton is in, as how far each
joint is from where it rests.

What the value of such an attribute means was read off the files, which say
no more: the middle frame of every clip holds the joints where the mesh rests
(all but one exactly, the last within a tenth of a unit), the entities give
the attributes a default of 0.5, and the genes give them values from -0.5 to
0.5. So the value is the default plus what the genes add, and picks a place
along the clip from its start at 0 to its end at 1, the middle being the face
as it is stored.

## Decals of the genes

The genes of a portrait also lay *decals* over its skin: wrinkles, crow's
feet, stubble, the colour of the lips, the eyebrows. Each names the part of a
portrait it is laid over, which an entity's game data says it is:

```
portrait_entity_user_data = { portrait_decal = { body_part = head } }
```

The loader reads that into `model.Part.DecalBodyPart`, and
`render.Model.LayDecals` lays the decals the genes pick over the parts of
their body part that are skin, as only the games' skin shader lays them. They
are laid the way the games' `AddDecals` and `BlendDecal` of
`jomini/portrait_decals.fxh` do it: in order of priority, those laid before
the skin colour first, each map of a decal blended over the part's by its
blend mode (overlay, replace, hard_light or multiply), the colour in linear
light, the normal maps by the games' overlay of normals, each as opaque as its
weight and the alpha of its diffuse map make it.

The games lay them for every pixel drawn, out of arrays of textures; a shader
of raylib's has too few texture units for the dozen or more a head carries.
So they are laid over the maps of the part instead, on the GPU, whenever they
change: one pass per decal and map into a target of half floats, and a last
pass back into a map like the part's own, which the part is drawn with until
the decals change again.

```go
uploaded.LayDecals([]render.Decal{{BodyPart: "head", Diffuse: wrinkles, DiffuseMode: render.BlendMultiply, Weight: 0.8}}, render.DefaultLook)
```

## Joining the models of a portrait

The games draw a portrait as several entities, which no asset file puts
together: the portrait settings of a game name a head and a body, and attach
the head to the body by joints, and the portrait accessories of
`gfx/portraits/accessories` draw further entities, such as the eyes, with the
pose of the head or the body:

```
joint_attachment = { parent_joint = "bn_sp_thoracic" child_joint = "head_root" }
male_eyes_normal = { entity = { entity = "male_eyes_entity" shared_pose_entity = head } }
```

`model.Model.Joint` and `Place` seat one model on another where their joints
meet. `model.Model.SharePose` adds the parts of an entity skinned to the same
bones, such as the eyes, to the model whose pose they share, with its
additive animations. `render.Model.Follow` joins a model's joints to another
model's from then on, so that a head stays on a body that its additive
animations make taller. A joint joined so takes the other's whole for its own
vertices, scale and all, but the bones below it hang from it without the
scale: read off the infant's body of Victoria 3, which shrinks the spine the
head hangs from to a third while the game draws the head large on it.

`render.Model.Hang` hangs a model rigidly from one joint of another, as a
portrait accessory with a `node` is: a sabre from `bn_l_prop` in the hand, a
pipe from a bone of the lips. Such a model's mesh has no skeleton and is made
in the space of the joint, and every pass draws it wherever the other model's
poses take that joint, turned and scaled with it.

The hair and the beards of Victoria 3 are rigged with the namespace of the rig
they were made on, their bones called `male_head_rig_01:bn_h_head` where the
head's are `bn_h_head`, and give the bones no vertex is skinned to, such as
the root and the neck, a matrix of zeros. `SharePose` names them as the head
does, and a part that shares a pose is posed by the head's bones of the same
names, each from its own resting place (`skin.Shared`), never by a chain of
its own bones, which those zeros break.

Where the neck of Victoria 3's head meets its body the two meshes share a
ring of vertices at the same places, whose normals the files have disagree
by up to a quarter of a right angle, which the light draws as a seam.
`model.Model.MatchNormals` gives the head the body's normals there. The
ring does not stay together either: the head's blend shapes and bones move
the neck, which the body does not follow, and in Europa Universalis 5 open a
crack the background shows through. The games dress their portraits over it.
`render.Model.Weld` draws the head's ring wherever the body's is drawn, as it
is posed and morphed, with its normals, so a portrait drawn bare shows no
seam. The rings are found by the open edges of the two meshes and matched
within a tolerance, since they do not always rest at the same places: a
woman's head of Europa Universalis 5 ends a little apart from her body, and
a boy is drawn with a man's head.

A portrait plays the states of its entities' `.asset` definitions, which go
on from one animation to the next by chance; `state.Machine` plays them, and
says which animation to pose a model in, and how far into it.

## Lighting a portrait as the games do

The games light a portrait with an environment of `gfx/portraits/environments`,
which pdx-parser-go's `portrait` package reads: a cubemap of its surroundings,
spot, point and directional lights, a shadow camera the shadows of the lights
are cast from, and the tonemap and colour settings every environment shares.
`render.Lighting`, set on a `Look`, is that, in the place of the renderer's
own studio of lights:

- `texture.DecodeCubeDDS` reads the six faces of a cubemap, and
  `Renderer.UploadEnvironment` lays them out as one picture of longitude and
  latitude, since raylib binds flat textures only, whose smaller copies stand
  in for the blur of a rough surface's reflection. The light it sheds on a
  surface is worked out once, as nine spherical harmonics.
- Each light fades over its falloff at the end of its radius, and a spot light
  over its cone, and a surface takes the lights and the environment as the
  games' `clausewitz/gfx/FX/cw/lighting.fxh` has it: diffuse light, GGX
  specular light and the environment's reflection along the direction a rough
  surface reflects the most. The viewer draws the depth of the models from
  the shadow camera first, and the lights marked as affected by shadow are
  darkened where that camera does not see a surface. Parts drawn with an
  effect the games give no shadow effect, such as `portrait_eye` and
  `portrait_hair_alpha`, which eyelashes are drawn with, cast none.
- `Lighting.Occlusion` darkens what is hidden from the light around it, the
  games' screen space ambient occlusion: the viewer draws the depth of the
  models from its camera, works the occlusion out from it and blurs it as
  jomini's `ssao.shader` does, and the shader lays it over the light before
  it is tonemapped, darkening each part towards a colour of its own, the
  texture of its slot 3 multiplied by its palette colour: a deep red for
  skin, black for clothes. The alpha of that texture weighs how much of the
  occlusion is laid on, as `ssao_apply.shader` weighs it, which leaves out
  the eyes and nostrils of Europa Universalis 5's heads.
- The light is then exposed, tonemapped by the ACES filmic curve as the
  games' `ToneMapAcesFilmic_Hill` does (`Lighting.ACES`) or by John Hable's as
  their `ToneMapUncharted2` does (`Lighting.Tonemap`), turned to gamma, and
  scaled in saturation and value and balanced, in the order of their
  `restorescene.shader`.

What the engine sets itself is not in any file: its fixed exposure, which
tonemap a portrait is drawn with, the points the ambient occlusion samples.
An application fits the exposure to the game's pictures, see
`Lighting.Exposure`. The game's portraits match the ACES curve, and show
skin as matte as it is with the specular strength of a properties map read
as sRGB and its roughness taken as it is stored rather than squared, which
widens the highlights.

`Viewer.LookFrom` stands the camera at a place, looking at a point, with a
field of view, as the games' portrait cameras of `gfx/portraits/cameras` are
written, and `Viewer.Pan` moves the view along.

## Highlighting what changes

An editor of portraits shows what a gene changes by highlighting it.
`render.Model.Snapshot` copies where the vertices of a model are drawn, as it
was last morphed and posed; `render.Movement` measures how far each moved
between two snapshots; and `render.Model.Highlight` tints the vertices by how
far, out of the furthest, in the colour `Look.Highlight` gives. Morphing the
model with the gene at its weakest and at its strongest, snapshotting each,
and morphing it back finds what the gene moves. The tint is carried in the
alpha of each vertex's colour, which every piece is uploaded with.

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

- The scale of a pdxmesh and an entity.

## License

MIT, see [LICENSE](LICENSE). The Targa decoder started as the one
of [pdx-flag-builder-go](https://github.com/kaiser-chris/pdx-flag-builder-go).
