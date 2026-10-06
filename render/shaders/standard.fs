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
// The game lights its portraits with an environment map and several lights of
// its own placement; here a key light, a fill light and a rim light stand in,
// which is enough to see the shape and the material of a model from any side.

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

// The palette colour the colour mask blends in, and the interval the mask is
// remapped into.
uniform vec3 paletteColor;
uniform vec2 colorMaskInterval;

// Where the camera is, for the highlights.
uniform vec3 viewPosition;

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

// The specular part of one light: GGX, with Schlick's approximation of the
// Fresnel term, which is what the games' physically based lighting uses.
vec3 specular(vec3 normal, vec3 towardsLight, vec3 towardsViewer, float roughness, vec3 reflectance)
{
    vec3 halfway = normalize(towardsLight + towardsViewer);

    float alpha = max(roughness * roughness, 0.002);
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

void main()
{
    vec2 uv = atlas > 0.5 ? fragTexCoord2 : fragTexCoord;

    vec4 diffuse = texture(texture0, uv);
    vec4 properties = texture(texture1, uv);

    if (cutout > 0.5 && diffuse.a < 0.5)
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
    if (usePalette > 0.5 && diffuse.a > 0.0)
    {
        float blend = mix(colorMaskInterval.x, colorMaskInterval.y, diffuse.a);
        color = mix(color, color * paletteColor, blend);
    }

    vec3 albedo = pow(color, vec3(gamma));

    // The back of a part seen from both sides faces the other way.
    vec3 normal = normalize(gl_FrontFacing ? fragNormal : -fragNormal);
    if (length(fragTangent) > 0.0)
    {
        mat3 tangentSpace = mat3(normalize(fragTangent), normalize(fragBitangent), normal);
        normal = normalize(tangentSpace * unpackNormal(texture(texture2, uv)));
    }

    float subsurface = properties.r;
    float specularStrength = properties.g;
    float metalness = noMetal > 0.5 ? 0.0 : properties.b;
    float roughness = clamp(properties.a, 0.04, 1.0);

    vec3 reflectance = mix(vec3(0.08 * specularStrength), albedo, metalness);
    vec3 diffuseColor = albedo * (1.0 - metalness);

    vec3 towardsViewer = normalize(viewPosition - fragPosition);

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
        light += colors[index] * specular(normal, directions[index], towardsViewer, roughness, reflectance);
    }

    finalColor = vec4(pow(clamp(light, 0.0, 1.0), vec3(1.0 / gamma)), blended > 0.5 ? diffuse.a : 1.0);
}
