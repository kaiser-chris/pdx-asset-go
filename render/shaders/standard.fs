#version 330

// The pixel stage every model is drawn with: the games' material model, lit by
// a simple studio of lights.
//
// The material is read the way the games' portrait shader reads it, from
// gfx/FX/jomini/portrait.shader of the jomini folder:
//
//   - The diffuse map holds the colour, and in its alpha how much of the
//     palette colour, such as a skin tone, is blended in, remapped into an
//     interval the entity's game data gives.
//   - The properties map holds the subsurface scattering mask in red, the
//     specular strength in green, the metalness in blue and the roughness in
//     alpha.
//   - The normal map holds x in green and y in alpha, upside down, as the
//     games' UnpackRRxGNormal of clausewitz/gfx/FX/cw/utility.fxh reads it.
//
// The game lights its portraits with an environment: a cubemap of their
// surroundings and several lights of its own placement, whose shadows it
// casts from a camera of its own, the light then tonemapped and graded. A
// Lighting of the renderer is that, which the shader lights by when it is
// given one; see environment.go. Without one, a key light, a fill light and a
// rim light stand in, which is enough to see the shape and the material of a
// model from any side.

in vec2 fragTexCoord;
in vec2 fragTexCoord2;
in vec3 fragPosition;
in vec3 fragNormal;
in vec3 fragTangent;
in vec3 fragBitangent;

// raylib binds the diffuse, specular and normal maps of a material to these.
uniform sampler2D texture0; // diffuse
uniform sampler2D texture1; // properties
uniform sampler2D texture2; // normal
uniform sampler2D texture3; // tint

// The portrait accessory of a part, which the entity's game data names: the
// mask whose four channels say where each pattern is drawn, the pattern of
// each channel with the surface it brings, and the colours of the palette.
// The renderer binds these to texture units of its own rather than to maps of
// the material, which are all spoken for.
uniform sampler2D accessoryMask;
uniform sampler2D accessoryPattern0;
uniform sampler2D accessoryPattern1;
uniform sampler2D accessoryPattern2;
uniform sampler2D accessoryPattern3;
uniform sampler2D accessoryNormal0;
uniform sampler2D accessoryNormal1;
uniform sampler2D accessoryNormal2;
uniform sampler2D accessoryNormal3;
uniform sampler2D accessoryProperties0;
uniform sampler2D accessoryProperties1;
uniform sampler2D accessoryProperties2;
uniform sampler2D accessoryProperties3;

// Whether the part is drawn with its accessory, where each channel's pattern
// is placed over the surface, as the zoom, the turn in radians and the offset
// of its layout, and the colour the palette holds for each channel.
uniform float accessory;
uniform vec4 accessoryLayout[4];
uniform vec3 accessoryColours[4];

// The palette colour the colour mask blends in, and the interval the mask is
// remapped into.
uniform vec3 paletteColor;
uniform vec2 colorMaskInterval;

// The colours of the eyes and the hair, which are blended into the parts drawn
// with the effects of eyes and hair as the skin colour is into skin, and
// which of the three a part takes, and what says where: 0 the skin colour by
// the diffuse map's alpha, 1 the eye colour by the same, 2 the hair colour by
// the blue of the normal map, 3 the hair colour over the whole of it.
uniform vec3 eyeColor;
uniform vec3 hairColor;
uniform float paletteKind;

// The colour laid over what Model.Highlight marks, and how strongly at most.
uniform vec4 highlightColor;
in float fragHighlight;

// Where the camera is, for the highlights.
uniform vec3 viewPosition;

// The environment a Lighting lights with, in the place of the studio below,
// when environmentLit is 1; see environment.go. Each light is its place and
// how far it reaches, its colour and over how much of that it fades, which
// way it shines and its kind (0 spot, 1 point, 2 directional), and for a spot
// the cosines of its inner and outer cone.
const int maxLights = 8;
uniform float environmentLit;
uniform float lightCount;
uniform vec4 lightPositionRadius[maxLights];
uniform vec4 lightColorFalloff[maxLights];
uniform vec4 lightDirectionKind[maxLights];
uniform vec4 lightCones[maxLights];

// Where the shadows of the lights marked in the third of their cones are cast
// from: whether there are any, the matrix that takes a place to where it lies
// in the depth the shadow camera saw, that depth, and how soft a shadow's edge
// is and how much of the light it takes away.
uniform float shadowed;
uniform mat4 lightSpace;
uniform sampler2D shadowMap;
uniform vec2 shadowSoftness;

// How far off the surface the shadow is looked up, in the units of the models:
// half a centimetre of a portrait, enough for the texels of its shadow map.
const float shadowOffset = 0.5;

// The ambient occlusion of a Lighting: whether there is any, the light left at
// each pixel of the picture, which the pixel is looked up in by its place,
// the size of the picture, and how much of what is hidden is darkened; then
// the colour each part is darkened towards, and whether that is multiplied by
// the part's palette colour. See occlusion.go.
uniform float occluded;
uniform sampler2D occlusionMap;
uniform vec2 occlusionResolution;
uniform float occlusionBlend;
uniform sampler2D occlusionColour;
uniform float occlusionTinted;
uniform float occlusionMapped;

// The cubemap of the surroundings, laid out as longitude and latitude in the
// games' coordinates, how many smaller copies of it there are, the nine
// spherical harmonics of the light it sheds, and how brightly it lights.
uniform sampler2D environment;
uniform float environmentLevels;
uniform vec3 irradiance[9];
uniform float environmentIntensity;

// The curve the light is turned into colours by: the six numbers of John
// Hable's filmic curve and the light it takes for white, and which curve it
// is, 0 none, 1 Hable's and 2 the ACES curve; then the value and the
// saturation the colours are scaled by and the exposure the light is, and the
// balance of the colours.
uniform vec4 tonemapCurve[2];
uniform vec3 grading;
uniform vec3 colorBalance;

// How the part is drawn, each 0 or 1: whether the palette colour is blended
// in, whether what the diffuse map's alpha leaves out is cut away, and
// whether the part is laid over what is behind it by that alpha, and
// whether the properties map's metalness is left out, and whether the
// textures are read by the second set of texture coordinates, as an atlas.
uniform float usePalette;
uniform float cutout;
uniform float blended;
uniform float noMetal;
uniform float atlas;

// How a part drawn with alpha to coverage is drawn: 0 it is not, 1 its solid
// pixels alone, 2 the rest of them alone, laid over what is there; and the
// alpha from which a pixel covers every sample of a pixel, and so is solid.
uniform float coverage;
const float coverageWhole = 0.98;

// Whether the diffuse map is overlaid with the tint, as the games colour the
// grey leaves of their trees.
uniform float tinted;

// Whether what is grey of the diffuse map is overlaid with a green of leaves,
// for a tree whose files name no tint. The games fall back to a tint of
// their own then, which the middle of Victoria 3's tree_tint_01 stands for.
// What has a colour of its own, such as bark, keeps it.
uniform float foliage;
const vec3 foliageTint = vec3(0.33, 0.35, 0.125);

out vec4 finalColor;

// The light is worked out in linear light and only turned back into what a
// screen shows at the end, the way the games do it.
const float gamma = 2.2;

const vec3 keyDirection = normalize(vec3(0.6, 0.8, -0.9));
const vec3 keyColor = vec3(1.0, 0.97, 0.92) * 2.4;
const vec3 fillDirection = normalize(vec3(-0.8, 0.2, -0.5));
const vec3 fillColor = vec3(0.55, 0.62, 0.75) * 0.8;
const vec3 rimDirection = normalize(vec3(0.0, 0.5, 1.0));
const vec3 rimColor = vec3(1.0) * 0.9;

// The light from around, from the sky above to the ground below, as the
// games' environment maps give it: the averages of the faces of Victoria
// 3's, the sky bluish, the horizon brighter, the ground nearly black.
const vec3 skyAmbient = vec3(0.13, 0.16, 0.2);
const vec3 horizonAmbient = vec3(0.2, 0.22, 0.24);
const vec3 groundAmbient = vec3(0.04, 0.04, 0.045);

// How much of a light the diffuse part of a material sends back, which with
// the lights above leaves a white surface facing the key light just short of
// white.
const float diffuseScale = 0.36;

const float pi = 3.14159265;

// Overlay blending, as the games' Overlay of clausewitz/gfx/FX/cw/utility.fxh:
// the base darkened where the blend is dark and lightened where it is light.
vec3 overlay(vec3 base, vec3 blend)
{
    return mix(2.0 * base * blend, 1.0 - 2.0 * (1.0 - base) * (1.0 - blend), step(0.5, base));
}

vec3 unpackNormal(vec4 sample)
{
    vec2 xy = vec2(sample.g * 2.0 - 1.0, 1.0 - sample.a * 2.0);

    return vec3(xy, sqrt(clamp(1.0 - dot(xy, xy), 0.0, 1.0)));
}

// The games lay a pattern over an accessory through the second set of texture
// coordinates, zoomed, turned and moved by the layout the variation names.
// Which order the engine does those in is not written anywhere it ships, so
// this is the plain reading of its notes.
vec2 patternUv(vec2 uv, vec4 placement)
{
    float sine = sin(placement.y);
    float cosine = cos(placement.y);

    vec2 scaled = uv * placement.x;

    return vec2(scaled.x * cosine - scaled.y * sine, scaled.x * sine + scaled.y * cosine) + placement.zw;
}

// accessoryChannel lays one channel of the mask over a pixel: the pattern's
// colour, in the colour the palette holds for that channel, and the surface
// the pattern brings, each as strongly as the channel covers the pixel.
//
// A pattern is a mask rather than a colour: its channels say which of its
// regions covers a pixel, and the palette holds the colour those regions are
// drawn in. The plain silk most of the shipped patterns are is one region that
// covers everything, which is red in the red channel of a mask as readily as
// it is white; multiplying the palette by it would draw every accessory red.
//
// The pattern is the surface of the accessory rather than a colour laid over
// it, so where it covers a pixel its own normal map and properties map are
// what is drawn there, and the maps of the mesh belong to the part the pattern
// has replaced. A channel that brings no surface of its own is bound the
// surface of the part itself, which leaves it as it was.
void accessoryChannel(inout vec3 color, inout vec3 surface, inout vec4 properties,
    float maskChannel, vec2 patternCoordinates, vec4 placement, vec3 paletteColour,
    sampler2D patterned, sampler2D patternedNormal, sampler2D patternedProperties)
{
    if (maskChannel <= 0.0)
    {
        return;
    }

    vec2 where = patternUv(patternCoordinates, placement);

    vec3 drawn = texture(patterned, where).rgb;
    float weight = maskChannel * max(max(drawn.r, drawn.g), drawn.b);

    if (weight <= 0.0)
    {
        return;
    }

    color = mix(color, paletteColour, weight);
    surface = normalize(mix(surface, unpackNormal(texture(patternedNormal, where)), weight));
    properties = mix(properties, texture(patternedProperties, where), weight);
}

// accessoryPaint lays the accessory of a part over a pixel, one channel of its
// mask after another. The patterns are sampled through the second set of
// texture coordinates, which is what the games lay them with.
void accessoryPaint(inout vec3 color, inout vec3 surface, inout vec4 properties, vec2 uv, vec2 patternCoordinates)
{
    vec4 mask = texture(accessoryMask, uv);

    accessoryChannel(color, surface, properties, mask.r, patternCoordinates, accessoryLayout[0], accessoryColours[0], accessoryPattern0, accessoryNormal0, accessoryProperties0);
    accessoryChannel(color, surface, properties, mask.g, patternCoordinates, accessoryLayout[1], accessoryColours[1], accessoryPattern1, accessoryNormal1, accessoryProperties1);
    accessoryChannel(color, surface, properties, mask.b, patternCoordinates, accessoryLayout[2], accessoryColours[2], accessoryPattern2, accessoryNormal2, accessoryProperties2);
    accessoryChannel(color, surface, properties, mask.a, patternCoordinates, accessoryLayout[3], accessoryColours[3], accessoryPattern3, accessoryNormal3, accessoryProperties3);
}

// The specular part of one light: GGX, with Schlick's approximation of the
// Fresnel term, which is what the games' physically based lighting uses, for
// a lobe as wide as alpha.
vec3 specular(vec3 normal, vec3 towardsLight, vec3 towardsViewer, float alpha, vec3 reflectance)
{
    vec3 halfway = normalize(towardsLight + towardsViewer);

    alpha = max(alpha, 0.002);
    float facing = max(dot(normal, halfway), 0.0);
    float denominator = facing * facing * (alpha * alpha - 1.0) + 1.0;
    float distribution = alpha * alpha / (pi * denominator * denominator);

    float k = alpha / 2.0;
    float lit = max(dot(normal, towardsLight), 0.0);
    float seen = max(dot(normal, towardsViewer), 0.001);
    float visibility = 1.0 / ((lit * (1.0 - k) + k) * (seen * (1.0 - k) + k));

    vec3 fresnel = reflectance + (1.0 - reflectance) * pow(1.0 - max(dot(halfway, towardsViewer), 0.0), 5.0);

    return distribution * visibility * fresnel * lit / 4.0;
}

// The specular part of one light the way the games work it out, in
// CalcSpecularBRDF of clausewitz/gfx/FX/cw/lighting.fxh with its simple
// lighting: GGX of a lobe remapped to no narrower than 0.03, so that a smooth
// surface does not show a pinpoint of light, Schlick's Fresnel term by the
// angle to the halfway vector, and their optimised visibility term; see
// environmentLight for the roughness it is given.
vec3 engineSpecular(vec3 normal, vec3 towardsLight, vec3 towardsViewer, float roughness, vec3 reflectance)
{
    vec3 halfway = normalize(towardsLight + towardsViewer);
    float facing = clamp(dot(normal, halfway), 0.0, 1.0);
    float lightHalf = clamp(dot(towardsLight, halfway), 0.0, 1.0);

    float alpha = mix(0.03, 1.0, roughness);
    float alpha2 = alpha * alpha;
    float f = (facing * alpha2 - facing) * facing + 1.0;
    float distribution = alpha2 / (pi * f * f);

    vec3 fresnel = reflectance + (1.0 - reflectance) * pow(1.0 - lightHalf, 5.0);

    float k = roughness * 0.5;
    float visibility = 0.25 / (lightHalf * lightHalf * (1.0 - k * k) + k * k);

    return distribution * fresnel * visibility;
}

// The games' coordinates are left handed, and the renderer's the mirror of
// them across x, so a direction is mirrored back to look up their cubemap.
vec3 gameDirection(vec3 direction)
{
    return vec3(-direction.x, direction.y, direction.z);
}

// The light the environment sheds on a surface facing a direction, from its
// nine spherical harmonics.
vec3 environmentIrradiance(vec3 normal)
{
    vec3 n = gameDirection(normal);

    return irradiance[0] * 0.282095
        + irradiance[1] * 0.488603 * n.y
        + irradiance[2] * 0.488603 * n.z
        + irradiance[3] * 0.488603 * n.x
        + irradiance[4] * 1.092548 * n.x * n.y
        + irradiance[5] * 1.092548 * n.y * n.z
        + irradiance[6] * 0.315392 * (3.0 * n.z * n.z - 1.0)
        + irradiance[7] * 1.092548 * n.x * n.z
        + irradiance[8] * 0.546274 * (n.x * n.x - n.y * n.y);
}

// The environment seen along a direction, blurred as much as a surface of a
// roughness blurs its reflection, from the picture of longitude and latitude
// it is laid out in.
vec3 environmentReflection(vec3 direction, float roughness)
{
    vec3 d = gameDirection(normalize(direction));
    vec2 where = vec2(atan(d.x, -d.z) / (2.0 * pi) + 0.5, acos(clamp(d.y, -1.0, 1.0)) / pi);

    return pow(textureLod(environment, where, roughness * environmentLevels).rgb, vec3(gamma));
}

// How strongly a light reaches a distance: fully, then fading to nothing over
// its falloff at the end of its radius.
float lightFalloff(float radius, float distanceToLight, float falloff)
{
    return clamp((radius - distanceToLight) / falloff, 0.0, 1.0);
}

// How much of a light marked as affected by shadow reaches the pixel: all of
// it where the shadow camera sees the pixel, less where something stands
// before it, averaged over a few texels around for a soft edge.
float shadowTerm(vec3 normal)
{
    if (shadowed < 0.5)
    {
        return 1.0;
    }

    // Looked up a little off the surface, along its normal, so that a surface
    // turned away from where the shadows are cast from does not shadow
    // itself in stripes of the texels it falls between.
    vec4 clip = lightSpace * vec4(fragPosition + normalize(fragNormal) * shadowOffset, 1.0);
    vec3 seen = clip.xyz / clip.w * 0.5 + 0.5;

    if (seen.x < 0.0 || seen.x > 1.0 || seen.y < 0.0 || seen.y > 1.0 || seen.z > 1.0)
    {
        return 1.0;
    }

    // A little nearer than the surface, so that it does not shadow itself.
    float bias = 0.0001;
    float lit = 0.0;

    for (int y = -2; y <= 2; y++)
    {
        for (int x = -2; x <= 2; x++)
        {
            float nearest = texture(shadowMap, seen.xy + vec2(x, y) * shadowSoftness.x).r;
            lit += seen.z - bias <= nearest ? 1.0 : 0.0;
        }
    }

    return mix(1.0, lit / 25.0, shadowSoftness.y);
}

// The light of the environment's lights and its cubemap on a surface, as the
// games' portrait shader adds them up: the light of each light, diffuse and
// specular, the light of the cubemap, and the light skin lets through.
vec3 environmentLight(vec3 normal, vec3 towardsViewer, vec3 diffuseColor, vec3 reflectance, float perceptual, float subsurface, vec3 albedo)
{
    // The games square what the properties map holds into the roughness
    // their lighting takes. Their portraits show skin as rough as it is
    // without the square, which widens the highlights; with it, the narrow
    // highlights of the lights at a glancing angle leave skin looking wet.
    float roughness = perceptual;
    vec3 light = vec3(0.0);
    float shadow = shadowTerm(normal);

    for (int index = 0; index < maxLights; index++)
    {
        if (float(index) >= lightCount)
        {
            break;
        }

        vec4 positionRadius = lightPositionRadius[index];
        vec4 colorFalloff = lightColorFalloff[index];
        vec4 directionKind = lightDirectionKind[index];

        vec3 towardsLight;
        float strength = 1.0;

        if (directionKind.w > 1.5)
        {
            towardsLight = -normalize(directionKind.xyz);
        }
        else
        {
            vec3 toLight = positionRadius.xyz - fragPosition;
            float distanceToLight = length(toLight);

            towardsLight = toLight / max(distanceToLight, 0.0001);
            strength = lightFalloff(positionRadius.w, distanceToLight, colorFalloff.w);

            if (directionKind.w < 0.5)
            {
                strength *= smoothstep(lightCones[index].y, lightCones[index].x, dot(-towardsLight, normalize(directionKind.xyz)));
            }
        }

        if (strength <= 0.0)
        {
            continue;
        }

        float lit = clamp(dot(normal, towardsLight), 0.0, 1.0) + 1e-5;
        vec3 color = colorFalloff.rgb * strength * (lightCones[index].z > 0.5 ? shadow : 1.0);

        light += color * diffuseColor * lit / pi;
        light += color * engineSpecular(normal, towardsLight, towardsViewer, roughness, reflectance) * lit;
    }

    // The environment lights the surface as the games' CalculateLightingFromIBL
    // has it: the light it sheds on a surface facing the normal, and its
    // reflection along the direction a rough surface reflects the most, a
    // little towards the normal from the mirror's, blurred by the roughness as
    // their BurleyToMipSimple does and faded as much as a rough surface's
    // microfacets turn the light away.
    light += environmentIrradiance(normal) / pi * diffuseColor * environmentIntensity;

    vec3 mirrored = reflect(-towardsViewer, normal);
    float smoothness = clamp(1.0 - roughness, 0.0, 1.0);
    vec3 dominant = normalize(mix(normal, mirrored, smoothness * (sqrt(smoothness) + roughness)));
    float facing = clamp(dot(normal, dominant), 0.0, 1.0);
    vec3 fresnel = reflectance + (1.0 - reflectance) * pow(1.0 - facing, 5.0);

    // Of the ten levels the games take a cubemap to have, the blur reaches
    // down to the third smallest.
    float blur = perceptual * (1.7 - 0.7 * perceptual) * 7.0 / 9.0;

    light += environmentReflection(dominant, blur) * fresnel / (roughness * roughness + 1.0) * environmentIntensity;

    // Skin glows a little of its own hue, at full brightness, where the
    // properties map's red says light goes through it: the games' fake
    // subsurface scattering.
    float brightest = max(max(albedo.r, albedo.g), albedo.b);
    vec3 hue = brightest > 0.0 ? albedo / brightest : vec3(0.0);
    light += hue * subsurface * 0.5 * diffuseColor;

    return light;
}

// One channel of John Hable's curve.
vec3 hable(vec3 x)
{
    float a = tonemapCurve[0].x;
    float b = tonemapCurve[0].y;
    float c = tonemapCurve[0].z;
    float d = tonemapCurve[0].w;
    float e = tonemapCurve[1].x;
    float f = tonemapCurve[1].y;

    return ((x * (a * x + c * b) + d * e) / (x * (a * x + b) + d * f)) - e / f;
}

// A colour as hue, saturation and value, and back.
vec3 toHSV(vec3 c)
{
    vec4 k = vec4(0.0, -1.0 / 3.0, 2.0 / 3.0, -1.0);
    vec4 p = mix(vec4(c.bg, k.wz), vec4(c.gb, k.xy), step(c.b, c.g));
    vec4 q = mix(vec4(p.xyw, c.r), vec4(c.r, p.yzx), step(p.x, c.r));
    float d = q.x - min(q.w, q.y);

    return vec3(abs(q.z + (q.w - q.y) / (6.0 * d + 1e-10)), d / (q.x + 1e-10), q.x);
}

vec3 fromHSV(vec3 c)
{
    vec3 p = abs(fract(c.xxx + vec3(1.0, 2.0 / 3.0, 1.0 / 3.0)) * 6.0 - 3.0);

    return c.z * mix(vec3(1.0), clamp(p - 1.0, 0.0, 1.0), c.y);
}

// The ACES filmic curve as Stephen Hill fits it, with the exposure bias of
// 1.8 the games' ToneMapAcesFilmic_Hill of jomini's posteffect_base.fxh
// gives it: the colour taken into the space the curve is fitted in, curved,
// and taken back. The matrices are written by their rows, as the games write
// them.
vec3 aces(vec3 color)
{
    vec3 v = color * 1.8;
    v = vec3(dot(vec3(0.59719, 0.35458, 0.04823), v),
             dot(vec3(0.07600, 0.90834, 0.01566), v),
             dot(vec3(0.02840, 0.13383, 0.83777), v));

    vec3 a = v * (v + 0.0245786) - 0.000090537;
    vec3 b = v * (0.983729 * v + 0.4329510) + 0.238081;
    v = a / b;

    return vec3(dot(vec3(1.60475, -0.53108, -0.07367), v),
                dot(vec3(-0.10208, 1.10813, -0.00605), v),
                dot(vec3(-0.00327, -0.07276, 1.07602), v));
}

// The light of an environment turned into the colours a screen shows, in the
// order of the games' restorescene.shader: exposed by a fixed exposure,
// tonemapped by the ACES curve or, as their ToneMapUncharted2 does, by
// John Hable's with its exposure bias of two, turned to gamma, and only then
// scaled in saturation and value and balanced, as their RestoreScene does.
vec3 grade(vec3 light)
{
    vec3 color = light * grading.z;

    if (tonemapCurve[1].w > 1.5)
    {
        color = aces(color);
    }
    else if (tonemapCurve[1].w > 0.5)
    {
        color = hable(2.0 * color) / hable(vec3(tonemapCurve[1].z));
    }

    color = pow(clamp(color, 0.0, 1.0), vec3(1.0 / gamma));

    vec3 hsv = toHSV(color);
    hsv.y *= grading.y;
    hsv.z *= grading.x;

    return clamp(fromHSV(hsv) * colorBalance, 0.0, 1.0);
}

// The palette colour of the part: the hair's, the eyes' or by default the
// skin's.
vec3 paletteTone()
{
    return paletteKind > 1.5 ? hairColor : paletteKind > 0.5 ? eyeColor : paletteColor;
}

// What the light of a pixel is multiplied by for the ambient occlusion, as
// the games' ssao_apply.shader lays it over theirs: towards the colour of the
// part as much as the pixel is hidden, and as much of that as the alpha of
// the colour says. A part without a colour of its own, as the games draw
// their attachments, is darkened towards black, wholly. The games read the
// colour as it is stored, as they do every texture of a numbered slot not
// marked as sRGB, and the palette colour is taken back into the linear light
// it multiplies in; see PaletteColour.
vec3 occlusionTint(vec2 uv)
{
    float left = texture(occlusionMap, gl_FragCoord.xy / occlusionResolution).r;
    float amount = clamp((1.0 - left) * occlusionBlend, 0.0, 1.0);

    vec4 sampled = occlusionMapped > 0.5 ? texture(occlusionColour, uv) : vec4(0.0, 0.0, 0.0, 1.0);
    amount *= sampled.a;

    vec3 colour = sampled.rgb;
    if (occlusionTinted > 0.5)
    {
        colour *= pow(paletteTone(), vec3(gamma));
    }

    return colour * amount + vec3(1.0 - amount);
}

void main()
{
    vec2 uv = atlas > 0.5 ? fragTexCoord2 : fragTexCoord;

    vec4 diffuse = texture(texture0, uv);
    vec4 properties = texture(texture1, uv);

    if (cutout > 0.5 && diffuse.a < 0.5)
    {
        discard;
    }

    // A part drawn with alpha to coverage is drawn solid where its alpha is
    // whole, and laid over what is there by its alpha elsewhere.
    bool whole = diffuse.a >= coverageWhole;
    if (coverage > 0.5 && coverage < 1.5 && !whole || coverage > 1.5 && (whole || diffuse.a < 1.0 / 255.0))
    {
        discard;
    }

    vec3 color = diffuse.rgb;

    // The games pick a colour along the tint for each tree, at random, and
    // some of them by the month as well; the middle of it stands for them.
    // The tint is overlaid with the diffuse map, which keeps the light and
    // dark of the leaves, as Crusader Kings 3 and Europa Universalis 5 do.
    if (tinted > 0.5)
    {
        color = overlay(textureLod(texture3, vec2(0.5), 0.0).rgb, color);
    }
    else if (foliage > 0.5)
    {
        float brightest = max(max(color.r, color.g), color.b);
        float saturation = brightest > 0.0 ? (brightest - min(min(color.r, color.g), color.b)) / brightest : 0.0;

        color = mix(overlay(foliageTint, color), color, clamp(saturation * 4.0, 0.0, 1.0));
    }

    // An alpha of exactly zero means no palette colour at all, which the
    // games use for what is not skin or hair, such as an earring.
    if (usePalette > 0.5)
    {
        vec3 tone = paletteTone();

        if (paletteKind > 2.5)
        {
            color *= tone;
        }
        else if (paletteKind > 1.5)
        {
            float blend = mix(colorMaskInterval.x, colorMaskInterval.y, texture(texture2, uv).b);
            color = mix(color, color * tone, blend);
        }
        else if (diffuse.a > 0.0)
        {
            float blend = mix(colorMaskInterval.x, colorMaskInterval.y, diffuse.a);
            color = mix(color, color * tone, blend);
        }
    }

    // The surface of the part, read before the accessory is laid over it: what
    // a pattern brings is in the tangent space of the mesh, so it takes the
    // place of the mesh's own normal map rather than of the normal itself.
    vec3 surface = unpackNormal(texture(texture2, uv));

    // The accessory colours and surfaces what the entity's game data calls a
    // pattern: every channel of its mask lays its own pattern over the part,
    // in the colour the palette holds for that channel, one channel after
    // another.
    if (accessory > 0.5)
    {
        accessoryPaint(color, surface, properties, uv, fragTexCoord2);
    }

    vec3 albedo = pow(color, vec3(gamma));

    // The back of a part seen from both sides faces the other way.
    vec3 normal = normalize(gl_FrontFacing ? fragNormal : -fragNormal);
    if (length(fragTangent) > 0.0)
    {
        mat3 tangentSpace = mat3(normalize(fragTangent), normalize(fragBitangent), normal);
        normal = normalize(tangentSpace * surface);
    }

    float subsurface = properties.r;
    float specularStrength = properties.g;
    float metalness = noMetal > 0.5 ? 0.0 : properties.b;
    float roughness = clamp(properties.a, 0.04, 1.0);

    vec3 reflectance = mix(vec3(0.08 * specularStrength), albedo, metalness);
    vec3 diffuseColor = albedo * (1.0 - metalness);

    vec3 towardsViewer = normalize(viewPosition - fragPosition);

    if (environmentLit > 0.5)
    {
        // The games take a quarter of the properties map's specular strength
        // as the reflectance of what is not metal. Their portraits show skin
        // as matte as it is when that strength is read as sRGB, as the
        // colour of the map is; read as it is stored, their own specular
        // lighting leaves skin looking wet.
        vec3 engineReflectance = mix(vec3(0.25 * pow(specularStrength, gamma)), albedo, metalness);
        vec3 light = environmentLight(normal, towardsViewer, diffuseColor, engineReflectance, clamp(properties.a, 0.0, 1.0), subsurface, albedo);

        if (occluded > 0.5)
        {
            light *= occlusionTint(uv);
        }

        vec3 drawn = grade(light);
        drawn = mix(drawn, highlightColor.rgb, clamp(fragHighlight, 0.0, 1.0) * highlightColor.a);

        finalColor = vec4(drawn, blended > 0.5 ? diffuse.a : 1.0);

        return;
    }

    vec3 ambient = normal.y > 0.0 ? mix(horizonAmbient, skyAmbient, normal.y) : mix(horizonAmbient, groundAmbient, -normal.y);
    vec3 light = ambient * diffuseColor;

    vec3 directions[3] = vec3[](keyDirection, fillDirection, rimDirection);
    vec3 colors[3] = vec3[](keyColor, fillColor, rimColor);

    for (int index = 0; index < 3; index++)
    {
        float lit = max(dot(normal, directions[index]), 0.0);

        // Skin lets light through and around: where the mask says so, the
        // light reaches a little past the edge of what faces it.
        float wrapped = max((dot(normal, directions[index]) + subsurface * 0.5) / (1.0 + subsurface * 0.5), 0.0);

        light += colors[index] * diffuseColor * mix(lit, wrapped, subsurface) * diffuseScale;
        light += colors[index] * specular(normal, directions[index], towardsViewer, roughness * roughness, reflectance);
    }

    vec3 shown = pow(clamp(light, 0.0, 1.0), vec3(1.0 / gamma));
    shown = mix(shown, highlightColor.rgb, clamp(fragHighlight, 0.0, 1.0) * highlightColor.a);

    finalColor = vec4(shown, blended > 0.5 ? diffuse.a : 1.0);
}
