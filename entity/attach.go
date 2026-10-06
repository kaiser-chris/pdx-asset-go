package entity

import (
	"path"
	"slices"
	"strings"

	"github.com/kaiser-chris/pdx-parser-go/asset"
	"github.com/kaiser-chris/pdx-parser-go/database"
	"github.com/kaiser-chris/pdx-parser-go/folders"
	"github.com/kaiser-chris/pdx-parser-go/report"
)

// An entity attaches others to points of its own, by name:
//
//	entity = {
//		name = "hub_entity"
//		locator = { name = "pos_1" position = { -16 0 12 } }
//		attach = { pos_1 = "academy_entity" }
//	}
//
// The name of an entity is global: the one attached can be defined in any
// asset file. The point is looked for the way the shipped files use them, in
// this order:
//
//   - a locator of the entity's own, or of an entity it clones, as above;
//   - a locator of its mesh file, which may hang from a bone, as the oars of
//     a ship of Europa Universalis 5 hang from its deck;
//   - a bone of its mesh, as a horse attaches a rider to its saddle.
//
// An entity can be nothing but its attachments, as the hubs of Victoria 3
// are, which draw no mesh and attach a whole town.
//
// The attachments of an entity it clones are its own as well. An attach
// block with a name replaces the one of the same name it clones, as the
// dragoon of Victoria 3 clones the hussar and swaps the sabre for a rifle,
// both attached as "weapon". A group of attachments the engine picks from
// at random is drawn with its first choice.

// maxAttachmentDepth is how deep attachments to attachments go before the
// loader gives up on them, far deeper than the shipped files go.
const maxAttachmentDepth = 16

// attachmentsOf collects what an entity attaches: its own attachments and
// those of the entities it clones, and the first choice of each group.
func attachmentsOf(definitions *asset.Assets, name string) []asset.Attachment {
	chain := definitions.CloneChain(name)

	var (
		attached []asset.Attachment
		groups   = map[string]bool{}
	)

	// The entity cloned furthest away comes first, so that what is nearer
	// replaces it.
	for _, entity := range slices.Backward(chain) {
		for _, attachment := range entity.Attachments {
			if attachment.ID != "" {
				attached = slices.DeleteFunc(attached, func(earlier asset.Attachment) bool { return earlier.ID == attachment.ID })
			}
		}

		attached = append(attached, entity.Attachments...)
	}

	// A group is picked from once, of the nearest entity that has it.
	for _, entity := range chain {
		for _, group := range entity.Groups {
			if groups[group.Name] || len(group.Choices) == 0 {
				continue
			}

			groups[group.Name] = true
			attached = append(attached, group.Choices[0].Attachments...)
		}
	}

	return slices.CompactFunc(attached, func(a, b asset.Attachment) bool { return a == b })
}

// attachmentPoint finds where on an entity a locator is: one of its own or of
// an entity it clones, one of its mesh file, or a bone of its mesh.
func attachmentPoint(definitions *asset.Assets, name, locator string, file *meshFile) (placement, bool) {
	for _, entity := range definitions.CloneChain(name) {
		for _, own := range entity.Locators {
			if strings.EqualFold(own.Name, locator) {
				return fromEntityLocator(own), true
			}
		}
	}

	if file == nil {
		return placement{}, false
	}

	for _, own := range file.locators {
		if !strings.EqualFold(own.Name, locator) {
			continue
		}

		placed := fromMeshLocator(own)

		// A locator that hangs from a bone is placed relative to it.
		if own.Parent != "" {
			if bone, ok := file.bone(own.Parent); ok {
				placed = placed.Then(bone)
			}
		}

		return placed, true
	}

	return file.bone(locator)
}

// bone is where a bone of the mesh is in the pose the mesh is stored in.
func (f *meshFile) bone(name string) (placement, bool) {
	for _, shape := range f.shapes {
		for _, bone := range shape.Skeleton {
			if strings.EqualFold(bone.Name, name) {
				return fromBone(bone)
			}
		}
	}

	return placement{}, false
}

// definitionsOf finds the definitions that hold an entity an asset file
// attaches: those of the entity that attaches it, those of the loader, and
// for a loader that looks by name, those of the asset files next to the one
// that attaches it.
func (l *Loader) definitionsOf(name string, within *asset.Assets, from database.Origin) (*asset.Assets, bool) {
	for _, definitions := range []*asset.Assets{within, l.assets} {
		if definitions.Entities.Has(name) {
			return definitions, true
		}
	}

	if !l.ByName {
		return nil, false
	}

	for _, definitions := range l.neighbours(path.Dir(from.File)) {
		if definitions.Entities.Has(name) {
			return definitions, true
		}
	}

	return nil, false
}

// neighbours reads the asset files of a folder, each on its own, the way a
// loose asset file is read, and keeps them for the next entity that looks
// there.
func (l *Loader) neighbours(folder string) []*asset.Assets {
	if read, ok := l.folders[folder]; ok {
		return read
	}

	files, _ := l.set.Files(folders.Folder{Path: folder, Extension: ".asset"})

	// What is wrong with the files is not this entity's problem: it is
	// reported when they are opened themselves.
	ignored := &report.Collector{}

	var read []*asset.Assets

	for _, parsed := range database.ParseFiles(files, ignored) {
		read = append(read, asset.Read(parsed.Document, parsed.File, ignored))
	}

	l.folders[folder] = read

	return read
}
