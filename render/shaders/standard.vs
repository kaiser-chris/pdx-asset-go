#version 330

// The vertex stage every model is drawn with. The geometry arrives converted
// to right handed coordinates already, so all this does is place it and hand
// the tangent frame on to the pixels, as the games' own vertex shaders do.

in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec2 vertexTexCoord2;
in vec3 vertexNormal;
in vec4 vertexTangent;

uniform mat4 mvp;
uniform mat4 matModel;
uniform mat4 matNormal;

out vec2 fragTexCoord;
out vec2 fragTexCoord2;
out vec3 fragPosition;
out vec3 fragNormal;
out vec3 fragTangent;
out vec3 fragBitangent;

void main()
{
    fragTexCoord = vertexTexCoord;
    fragTexCoord2 = vertexTexCoord2;
    fragPosition = vec3(matModel * vec4(vertexPosition, 1.0));
    fragNormal = normalize(vec3(matNormal * vec4(vertexNormal, 0.0)));

    // A mesh without tangents has zeros here, which the pixel stage takes as
    // a sign to leave the normal map out.
    vec3 tangent = vec3(matNormal * vec4(vertexTangent.xyz, 0.0));
    fragTangent = length(tangent) > 0.0 ? normalize(tangent) : vec3(0.0);

    // The fourth number of a tangent says which way round the bitangent goes.
    fragBitangent = cross(fragNormal, fragTangent) * vertexTangent.w;

    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
