package entity

import (
	"fmt"
	"io"
	"os"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/report"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// An entity's mesh names the animations it can play, each a .anim file, and
// may import a whole set of them made for another mesh's skeleton:
//
//	pdxmesh = {
//		name = "horse_cart_mesh"
//		animation = { id = "moving" type = "horse_cart_moving.anim" }
//		import = { type = skeletal_animation_set name = "common_body" }
//	}
//
// Listing them reads the head of each file rather than the whole of it. One
// body of Crusader Kings 3 can play seven hundred and sixty five animations,
// which together are near five hundred megabytes, so reading them all to say
// what they are is out of the question; the head of a file is its frames, its
// rate and its joints, which is all a list needs. See the anim package.

// animationsOf lists the animations the mesh of an entity can play, and adds
// them to the model. An animation whose file cannot be found or read is left
// out and reported.
func (l *Loader) animationsOf(job *loading, definitions *asset.Assets, entity *asset.Entity, definition *asset.Mesh, number int) {
	for _, animation := range definitions.AnimationsOf(definition) {
		head, file, err := l.readAnimationHead(animation)
		if err != nil {
			job.collector.Addf(report.SeverityWarning, animation.Origin.Source, animation.Origin.Path, animation.Origin.Line, 0,
				"entity "+entity.Key, "animation %s of pdxmesh %s: %v; left out", animation.ID, definition.Key, err)

			continue
		}

		job.model.Animations = append(job.model.Animations, model.Animation{
			ID:         animation.ID,
			Entity:     entity.Key,
			Attachment: number,
			Mesh:       definition.Key,
			File:       file,
			FPS:        head.FPS,
			Frames:     head.Frames,
			Joints:     len(head.Joints),
			Seconds:    head.Duration(),
		})
	}
}

// readAnimationHead reads what an animation is, without its samples, and says
// where its file was found. A rig too large for the head of a file is read
// from the whole of it.
func (l *Loader) readAnimationHead(animation asset.MeshAnimation) (*anim.Animation, string, error) {
	relative := asset.Resolve(animation.Origin, animation.File)
	if relative == "" {
		return nil, "", fmt.Errorf("no file is named")
	}

	found, ok := l.set.Find(relative)
	if !ok {
		found, ok = l.byName(animation.Origin, animation.File)
	}

	if !ok {
		return nil, "", fmt.Errorf("%s is in none of the folders", relative)
	}

	opened, err := os.Open(found.Path)
	if err != nil {
		return nil, "", err
	}
	defer opened.Close()

	prefix := make([]byte, anim.HeadBytes)

	size, err := io.ReadFull(opened, prefix)
	if err != nil && size == 0 {
		return nil, "", err
	}

	head, err := anim.ReadHead(prefix[:size])
	if err == nil {
		return head, found.Relative, nil
	}

	// A file whose joints do not fit in a head is read whole, which no rig
	// the games ship needs.
	if size < anim.HeadBytes {
		return nil, "", err
	}

	data, readErr := os.ReadFile(found.Path)
	if readErr != nil {
		return nil, "", readErr
	}

	whole, readErr := anim.Read(data)
	if readErr != nil {
		return nil, "", readErr
	}

	return whole, found.Relative, nil
}

// ReadAnimation reads a whole animation file, samples and all, given its path
// below the game's root, such as model.Animation.File gives. Listing reads
// only the head of each file; this is for playing one, whose samples move the
// geometry.
func (l *Loader) ReadAnimation(relative string) (*anim.Animation, error) {
	found, ok := l.set.Find(relative)
	if !ok {
		return nil, fmt.Errorf("%s is in none of the folders", relative)
	}

	data, err := os.ReadFile(found.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", found.Path, err)
	}

	return anim.Read(data)
}
