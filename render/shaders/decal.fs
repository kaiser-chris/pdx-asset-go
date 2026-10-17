#version 330

// The pass that lays the decals of a portrait's genes over the skin, one map
// of a part at a time and one decal at a time, in the part's own texture
// coordinates. It draws over the whole of the map: every pixel of the target
// is the pixel of the map at the same place.
//
// The decals are blended the way the games' AddDecals and BlendDecal do it,
// from jomini/portrait_decals.fxh and jomini/texture_decals_base.fxh of the
// jomini folder. The games lay them over the maps of the part as they are
// sampled, for every pixel drawn; here they are laid over the maps
// themselves, once, which comes to the same thing at the size of the map.
//
// What is laid is kept in a target of half floats between decals, as the
// games keep it in the registers of their shader: the colour in linear light,
// which their diffuse maps are sampled in, and the normal as a direction,
// unpacked. The last pass packs it back into a map like the part's own.

// The map the decals have been laid over so far, of the size of the target,
// and the part's own map, which the first pass reads and the last keeps the
// channels of that decals do not touch.
uniform sampler2D laid;
uniform sampler2D own;

// The decal laid by a pass of stage 1: its diffuse map, whose alpha makes it
// more or less opaque wherever it is, and its map of the kind laid.
uniform sampler2D decalDiffuse;
uniform sampler2D decalMap;

// The size of the target, in pixels.
uniform vec2 size;

// The stage, the kind and the mode are whole numbers, handed over as floats
// the way raylib hands its uniforms over, and rounded.
//
// What the pass does: 0 reads the part's own map, 1 lays a decal, 2 is the
// turn from the decals laid before the skin colour to those laid after, and
// 3 packs what was laid into a map like the part's own.
uniform float stage;

// The kind of map laid: 0 the diffuse map, 1 the normal map, 2 the
// properties map.
uniform float kind;

// How the decal of a pass of stage 1 is laid: whether it has a diffuse map
// and a map of the kind laid, how it is blended, as the games number their
// blend modes, and how opaque the genes make it.
uniform float hasDiffuse;
uniform float hasMap;
uniform float mode;
uniform float weight;

// The skin colour, in linear light, and the interval the colour mask is
// remapped into, which the turn of stage 2 blends into the diffuse map
// before the decals laid after it.
uniform vec3 paletteColor;
uniform vec2 colorMaskInterval;

out vec4 finalColor;

int stageOf()
{
    return int(stage + 0.5);
}

int kindOf()
{
    return int(kind + 0.5);
}

int modeOf()
{
    return int(mode + 0.5);
}

const float gamma = 2.2;

// The blend modes, numbered as the games' BLEND_MODE_ defines.
const int modeOverlay = 0;
const int modeReplace = 1;
const int modeHardLight = 2;
const int modeMultiply = 3;

// The games' OverlayDecal and HardLightDecal.
float overlayDecal(float target, float blend)
{
    return target > 0.5 ? 1.0 - 2.0 * (1.0 - target) * (1.0 - blend) : 2.0 * target * blend;
}

float hardLightDecal(float target, float blend)
{
    return blend > 0.5 ? 1.0 - 2.0 * (1.0 - target) * (1.0 - blend) : 2.0 * target * blend;
}

// The games' BlendDecal.
vec4 blendDecal(int blendMode, vec4 target, vec4 blend, float strength)
{
    vec4 result = vec4(0.0);

    if (blendMode == modeOverlay)
    {
        result = vec4(overlayDecal(target.r, blend.r), overlayDecal(target.g, blend.g), overlayDecal(target.b, blend.b), overlayDecal(target.a, blend.a));
    }
    else if (blendMode == modeReplace)
    {
        result = blend;
    }
    else if (blendMode == modeHardLight)
    {
        result = vec4(hardLightDecal(target.r, blend.r), hardLightDecal(target.g, blend.g), hardLightDecal(target.b, blend.b), hardLightDecal(target.a, blend.a));
    }
    else if (blendMode == modeMultiply)
    {
        result = target * blend;
    }

    return mix(target, result, strength);
}

// The games' UnpackRRxGNormal: x in green and y in alpha, upside down.
vec3 unpackNormal(vec4 sample)
{
    vec2 xy = vec2(sample.g * 2.0 - 1.0, 1.0 - sample.a * 2.0);

    return vec3(xy, sqrt(clamp(1.0 - dot(xy, xy), 0.0, 1.0)));
}

// The games' UnpackDecalNormal, which unpacks the normal of a decal as
// strong as the decal is, leaving out what is nearly flat: the errors of
// compressing many decals would add up where they were meant to be flat.
vec3 unpackDecalNormal(vec4 sample, float strength)
{
    vec2 xy = vec2(sample.g * 2.0 - 1.0, 1.0 - sample.a * 2.0);

    float filtered = smoothstep(0.0004, 0.0004 + 0.05 * 0.05, dot(xy, xy));
    xy *= strength * filtered;

    return vec3(xy, sqrt(clamp(1.0 - dot(xy, xy), 0.0, 1.0)));
}

// read reads the part's own map into what is laid: the colour in linear
// light with the colour mask as it is, the normal as a direction, and the
// properties as they are.
vec4 read(vec4 sample)
{
    if (kindOf() == 0)
    {
        return vec4(pow(sample.rgb, vec3(gamma)), sample.a);
    }

    if (kindOf() == 1)
    {
        return vec4(unpackNormal(sample), 0.0);
    }

    return sample;
}

// lay lays one decal over what is laid, as AddDecals does for each decal.
// The diffuse map's alpha makes the decal less opaque wherever it is, for
// every map of it, and the colour mask in the alpha of what is laid is left
// as it is: the games blend the colour alone.
vec4 lay(vec4 target, vec2 uv)
{
    float strength = weight;
    vec4 diffuse = vec4(1.0);

    if (hasDiffuse > 0.5)
    {
        diffuse = textureLod(decalDiffuse, uv, 0.0);
        strength *= diffuse.a;
    }

    if (kindOf() == 0)
    {
        if (hasDiffuse < 0.5)
        {
            return target;
        }

        vec4 colour = vec4(pow(diffuse.rgb, vec3(gamma)), diffuse.a);

        return vec4(blendDecal(modeOf(), vec4(target.rgb, 0.0), colour, strength).rgb, target.a);
    }

    if (hasMap < 0.5)
    {
        return target;
    }

    vec4 sample = textureLod(decalMap, uv, 0.0);

    if (kindOf() == 1)
    {
        vec3 normal = unpackDecalNormal(sample, strength);

        // The games' overlay of a normal adds the slopes and multiplies the
        // heights, BLEND_MODE_OVERLAY_NORMAL, which their shader puts in the
        // place of the overlay of colours.
        if (modeOf() == modeOverlay)
        {
            vec3 overlaid = vec3(target.xy + normal.xy, target.z * normal.z);

            return vec4(mix(target.xyz, overlaid, strength), 0.0);
        }

        return vec4(blendDecal(modeOf(), vec4(target.xyz, 0.0), vec4(normal, 0.0), strength).xyz, 0.0);
    }

    return blendDecal(modeOf(), target, sample, strength);
}

// turn is the turn from the decals laid before the skin colour to those laid
// after: AddDecals ends by normalising the normal, and the skin colour is
// blended into the colour as the games' GetColorMaskColorBLend does, where
// the colour mask is not exactly 0.
vec4 turn(vec4 target)
{
    if (kindOf() == 1)
    {
        return vec4(normalize(target.xyz), 0.0);
    }

    if (kindOf() == 0 && target.a > 0.0)
    {
        float blend = mix(colorMaskInterval.x, colorMaskInterval.y, target.a);

        return vec4(mix(target.rgb, target.rgb * paletteColor, blend), target.a);
    }

    return target;
}

// pack packs what was laid into a map like the part's own: the colour back
// out of linear light, and the normal normalised, as AddDecals leaves it,
// into the green and alpha of the part's own normal map.
vec4 pack(vec4 target, vec4 sample)
{
    if (kindOf() == 0)
    {
        return vec4(pow(clamp(target.rgb, 0.0, 1.0), vec3(1.0 / gamma)), target.a);
    }

    if (kindOf() == 1)
    {
        vec3 normal = normalize(target.xyz);

        return vec4(sample.r, normal.x * 0.5 + 0.5, sample.b, (1.0 - normal.y) * 0.5);
    }

    return clamp(target, 0.0, 1.0);
}

void main()
{
    vec2 uv = gl_FragCoord.xy / size;
    ivec2 texel = ivec2(gl_FragCoord.xy);

    if (stageOf() == 0)
    {
        finalColor = read(textureLod(own, uv, 0.0));
    }
    else if (stageOf() == 1)
    {
        finalColor = lay(texelFetch(laid, texel, 0), uv);
    }
    else if (stageOf() == 2)
    {
        finalColor = turn(texelFetch(laid, texel, 0));
    }
    else
    {
        finalColor = pack(texelFetch(laid, texel, 0), textureLod(own, uv, 0.0));
    }
}
