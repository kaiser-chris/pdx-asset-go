#version 330

// The games' screen space ambient occlusion, as jomini's ssao.shader works it
// out and blurs it; see occlusion.go. The first pass compares the depth of
// each pixel with that of points on a disc around it, turned at random from
// one pixel to the next, and keeps how much light is left in red: one where
// as much is nearer as further, more where the surface curves away, less in a
// crease. The second blurs that along one axis, held back where the depth
// changes, so that it does not run over the edge of the nose onto the cheek.
//
// Both find what they read by the place of the pixel they draw, so whatever
// covers the picture draws them.

in vec2 fragTexCoord;
in vec4 fragColor;

out vec4 finalColor;

// Which pass this is, 0 the occlusion and 1 the blur, and for the blur the
// axis, 0 across and 1 down.
uniform vec2 pass;

// The size of the picture, and the near and far clip planes its depth was
// drawn between.
uniform vec2 resolution;
uniform vec2 planes;

// The depth the camera saw, and for the blur the occlusion it blurs.
uniform sampler2D depthMap;
uniform sampler2D blurSource;

// The radius around a pixel at a depth of one and the most it can be, how
// much nearer a point has to be to hide the pixel fully and how much nearer
// it can be and still hide it; then the scale, the bias and the power the
// light left is shaped by, and how many samples there are.
uniform vec4 shape;
uniform vec4 apply;

const int maxSamples = 16;
uniform vec3 discSamples[maxSamples];

// How many pixels apart the blur's are, the power a difference in depth
// holds them back by, and how many it takes each way, with the one in the
// middle; and the weight of each, that one first.
const int maxWeights = 5;
uniform vec4 blur;
uniform float blurWeights[maxWeights];

// How far from the camera the surface a pixel shows is, from the depth the
// graphics card keeps of it.
float viewDepth(vec2 uv)
{
    float depth = texture(depthMap, uv).r * 2.0 - 1.0;

    return 2.0 * planes.x * planes.y / (planes.y + planes.x - depth * (planes.y - planes.x));
}

// A number from 0 to 1 that changes from one pixel to the next as if at
// random, the same every time for the same pixel.
float random(vec2 pixel)
{
    return fract(sin(dot(pixel, vec2(12.9898, 78.233))) * 43758.5453);
}

float occlusion(vec2 uv)
{
    float angle = random(floor(gl_FragCoord.xy)) * 6.2831853;
    vec2 turn = vec2(cos(angle), sin(angle));
    mat2 rotate = mat2(turn.x, -turn.y, turn.y, turn.x);

    float depth = viewDepth(uv);

    // The pixel itself counts as half hidden.
    float hidden = 0.5;

    // The radius shrinks the further away the surface is.
    float radius = min(shape.x / depth, shape.y);
    int count = int(apply.w);

    for (int index = 0; index < maxSamples; index++)
    {
        if (index >= count)
        {
            break;
        }

        vec2 point = rotate * discSamples[index].xy;
        point.x *= resolution.y / resolution.x;

        float sphere = discSamples[index].z * shape.z;
        float difference = depth - viewDepth(uv + point * radius);

        // How much of the sphere over the point is hidden, and none by what
        // is so much nearer that it stands in front instead.
        float covered = clamp(difference / max(sphere, 1e-4) + 0.5, 0.0, 1.0);
        covered *= clamp((shape.w - difference) / shape.w, 0.0, 1.0);

        hidden += covered;
    }

    float left = 1.0 - hidden / float(count + 1);

    return pow(max(apply.x * (left + apply.y), 0.0), apply.z);
}

float blurred(vec2 uv)
{
    float depth = viewDepth(uv);
    float sum = texture(blurSource, uv).r * blurWeights[0];
    float total = blurWeights[0];

    vec2 step = vec2(1.0 - pass.y, pass.y) * blur.x / resolution;
    int count = int(blur.z);

    for (int index = 1; index < maxWeights; index++)
    {
        if (index >= count)
        {
            break;
        }

        for (int side = -1; side <= 1; side += 2)
        {
            vec2 at = uv + step * float(index * side);
            float weight = blurWeights[index] / pow(1.0 + abs(depth - viewDepth(at)), blur.y);

            sum += texture(blurSource, at).r * weight;
            total += weight;
        }
    }

    return sum / max(total, 1e-6);
}

void main()
{
    vec2 uv = gl_FragCoord.xy / resolution;
    float value = pass.x < 0.5 ? occlusion(uv) : blurred(uv);

    finalColor = vec4(value, 0.0, 0.0, 1.0);
}
