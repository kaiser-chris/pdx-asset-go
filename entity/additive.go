package entity

import (
	"fmt"
	"os"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/report"

	"github.com/kaiser-chris/pdx-asset-go/anim"
	"github.com/kaiser-chris/pdx-asset-go/model"
)

// A mesh names its additive animations beside its animations:
//
//	pdxmesh = {
//		name = "male_head_mesh"
//		additive_animation = { id = "head_chin_forward" type = "male_head_chin_forward.anim" }
//	}
//
// and an entity the attributes that play them:
//
//	attribute = { name = "head_chin_forward" additive_animation = "head_chin_forward" default = 0.5 }
//
// An additive animation is a short clip, seven frames in every one Victoria 3
// ships, whose middle frame holds the joints it moves where the mesh rests,
// and whose ends move them as far as they go either way. They are read whole
// when the entity is loaded, since they are small and the attributes that
// play them can change at any time.

// additiveOf reads the additive animations of a pdxmesh, once for every time
// it is drawn. One whose file cannot be found or read is reported and left
// out.
func (l *Loader) additiveOf(job *loading, definition *asset.Mesh, subject string) []model.Additive {
	var found []model.Additive

	for _, animation := range definition.AdditiveAnimations {
		relative := asset.Resolve(definition.Origin(), animation.File)

		read, ok := job.additive[relative]
		if !ok {
			read = l.readAdditive(definition, animation, relative)

			if job.additive == nil {
				job.additive = map[string]additiveFile{}
			}

			job.additive[relative] = read
		}

		if read.err != nil {
			origin := definition.Origin()
			job.collector.Addf(report.SeverityWarning, origin.Source, origin.Path, origin.Line, 0, subject,
				"additive animation %s of pdxmesh %s: %v; left out", animation.ID, definition.Key, read.err)

			continue
		}

		found = append(found, model.Additive{ID: animation.ID, Animation: read.animation})
	}

	return found
}

// additiveFile is an additive animation file that has been read, or why it
// could not be.
type additiveFile struct {
	animation *anim.Animation
	err       error
}

// readAdditive finds and reads one additive animation file.
func (l *Loader) readAdditive(definition *asset.Mesh, animation asset.Animation, relative string) additiveFile {
	if relative == "" {
		return additiveFile{err: fmt.Errorf("no file is named")}
	}

	found, ok := l.set.Find(relative)
	if !ok {
		found, ok = l.byName(definition.Origin(), animation.File)
	}

	if !ok {
		return additiveFile{err: fmt.Errorf("%s is in none of the folders", relative)}
	}

	data, err := os.ReadFile(found.Path)
	if err != nil {
		return additiveFile{err: err}
	}

	read, err := anim.Read(data)
	if err != nil {
		return additiveFile{err: fmt.Errorf("read %s: %w", found.Path, err)}
	}

	return additiveFile{animation: read}
}
