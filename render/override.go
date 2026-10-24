package render

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/kaiser-chris/pdx-asset-go/texture"
)

// SkinMaps are the maps a gene's texture override gives the skin of a body
// part, such as Europa Universalis 5's smooth faces of gene_complexion. A nil
// map keeps the part's own.
type SkinMaps struct {
	Diffuse, Normal, Properties *texture.Image
}

// OverrideSkin draws the skin of a body part, such as the head, with other
// maps: each part of the body part whose effect is one of the games' skin
// effects, as Model.LayDecals finds them, for it is the skin the games'
// texture_override replaces. The decals laid over the part are laid over
// the new maps, at once, with the look they were last laid with. Overriding
// with no maps draws the parts with their own again.
//
// It has to run on the thread that owns the OpenGL context, after the model
// has been uploaded, and outside the drawing of a viewer.
func (m *Model) OverrideSkin(bodyPart string, maps SkinMaps) error {
	images := [decalMaps]*texture.Image{maps.Diffuse, maps.Normal, maps.Properties}

	for index := range m.parts {
		part := &m.parts[index]
		if part.decalBodyPart != bodyPart || !part.look.palette || part.look.paletteKind != paletteSkin {
			continue
		}

		var base [decalMaps]rl.Texture2D

		for kind, image := range images {
			done, err := m.texture(image, part.uploadedMaps[kind])
			if err != nil {
				return fmt.Errorf("model %s: %w", m.Name, err)
			}

			base[kind] = done
		}

		if base == part.baseMaps {
			continue
		}

		part.baseMaps = base

		if part.decals == nil || len(part.decals.drawn) == 0 {
			for kind, slot := range decalMaterialMaps {
				rl.SetMaterialTexture(part.material, slot, base[kind])
			}

			continue
		}

		// The decals are laid again over the new maps.
		laid := part.decals
		decals, look := laid.drawn, laid.look
		laid.drawn = nil

		if err := m.layPart(part, decals, look); err != nil {
			return fmt.Errorf("model %s: %w", m.Name, err)
		}
	}

	return nil
}
