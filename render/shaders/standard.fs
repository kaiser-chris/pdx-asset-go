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
//   - The normal map holds x in red and alpha, whichever of the two is not
//     white, and y in green.
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

// The palette colour the colour mask blends in, and the interval the mask is
// remapped into.
uniform vec3 paletteColor;
uniform vec2 colorMaskInterval;

// Where the camera is, for the highlights.
uniform vec3 viewPosition;

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
const vec3 ambient = vec3(0.16, 0.17, 0.2);

// How much of a light the diffuse part of a material sends back, which with
// the lights above leaves a white surface facing the key light just short of
// white.
const float diffuseScale = 0.36;

const float pi = 3.14159265;

vec3 unpackNormal(vec4 sample)
{
    vec2 xy = vec2(sample.r * sample.a, sample.g) * 2.0 - 1.0;

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
    vec4 diffuse = texture(texture0, fragTexCoord);
    vec4 properties = texture(texture1, fragTexCoord);

    vec3 color = diffuse.rgb;

    // An alpha of exactly zero means no palette colour at all, which the
    // games use for what is not skin or hair, such as an earring.
    if (diffuse.a > 0.0)
    {
        float blend = mix(colorMaskInterval.x, colorMaskInterval.y, diffuse.a);
        color = mix(color, color * paletteColor, blend);
    }

    vec3 albedo = pow(color, vec3(gamma));

    vec3 normal = normalize(fragNormal);
    if (length(fragTangent) > 0.0)
    {
        mat3 tangentSpace = mat3(normalize(fragTangent), normalize(fragBitangent), normal);
        normal = normalize(tangentSpace * unpackNormal(texture(texture2, fragTexCoord)));
    }

    float subsurface = properties.r;
    float specularStrength = properties.g;
    float metalness = properties.b;
    float roughness = clamp(properties.a, 0.04, 1.0);

    vec3 reflectance = mix(vec3(0.08 * specularStrength), albedo, metalness);
    vec3 diffuseColor = albedo * (1.0 - metalness);

    vec3 towardsViewer = normalize(viewPosition - fragPosition);

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

    finalColor = vec4(pow(clamp(light, 0.0, 1.0), vec3(1.0 / gamma)), 1.0);
}
