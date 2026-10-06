package anim

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gameFolder is the game to read, or none.
func gameFolder(t *testing.T) string {
	t.Helper()

	game := os.Getenv("PDX_GAME_DIR")
	if game == "" {
		t.Skip("set PDX_GAME_DIR to the game folder to run this test")
	}

	return game
}

// animationFiles are the .anim files below a folder.
func animationFiles(t *testing.T, folder string) []string {
	t.Helper()

	var found []string

	err := filepath.WalkDir(folder, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".anim") {
			found = append(found, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(found) == 0 {
		t.Skip("the folder holds no animations")
	}

	return found
}

// TestReadInstalledAnimations reads every animation of the game. None may
// fail: the files are all written the same way, so a failure is a gap in this
// package. What they hold is counted and logged.
func TestReadInstalledAnimations(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}

	files := animationFiles(t, gameFolder(t))

	var (
		read, uniform, poses int
		joints, frames       int
		longest              float64
		longestFile          string
		rates                = map[float64]int{}
	)

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s: %v", file, err)

			continue
		}

		animation, err := Read(data)
		if err != nil {
			t.Errorf("%s: %v", file, err)

			continue
		}

		read++
		joints += len(animation.Joints)
		frames += animation.Frames

		if animation.ScaleWidth == 1 {
			uniform++
		}

		if animation.Changes() == 0 {
			poses++
		}

		if seconds := animation.Duration(); seconds > longest {
			longest, longestFile = seconds, file
		}

		// Every pose of every joint of the first and last frames must be
		// there to be read, with a rotation of unit length.
		for joint := range animation.Joints {
			for _, frame := range []int{0, animation.Frames - 1} {
				pose := animation.Pose(joint, frame)

				length := 0.0
				for axis := range 4 {
					length += float64(pose.Rotation[axis]) * float64(pose.Rotation[axis])
				}

				if math.Abs(math.Sqrt(length)-1) > 1e-3 {
					t.Errorf("%s: joint %s of frame %d turns by %v, which is not of unit length",
						file, animation.Joints[joint].Name, frame, pose.Rotation)
				}
			}
		}

		// The rate a file was made at is a whole number for most of them.
		if rate := animation.Rate(); rate > 0 && math.Abs(rate-math.Round(rate)) < 0.01 {
			rates[math.Round(rate)]++
		}
	}

	t.Logf("read %d animations of %d joints and %d frames; %d scale every axis alike, %d change nothing",
		read, joints, frames, uniform, poses)
	t.Logf("the longest runs %.1fs: %s", longest, longestFile)
	t.Logf("whole frame rates: %v", rates)
}

// TestReadInstalledHeads reads the head of every animation of the game from
// its first HeadBytes bytes, which must give the same joints and frames as
// reading the whole file. Every animation of one body of Crusader Kings 3 is
// hundreds of megabytes together, so this is how the loader lists them.
func TestReadInstalledHeads(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}

	files := animationFiles(t, gameFolder(t))

	var read, cut int

	for _, file := range files {
		opened, err := os.Open(file)
		if err != nil {
			t.Errorf("%s: %v", file, err)

			continue
		}

		prefix := make([]byte, HeadBytes)
		size, err := opened.Read(prefix)
		opened.Close()

		if err != nil && size == 0 {
			t.Errorf("%s: %v", file, err)

			continue
		}

		head, err := ReadHead(prefix[:size])
		if err != nil {
			// A rig whose joints do not fit in the prefix is reported
			// rather than read short, which the loader answers by reading
			// the whole file.
			if strings.Contains(err.Error(), "cut off") {
				cut++

				continue
			}

			t.Errorf("%s: %v", file, err)

			continue
		}

		read++

		if head.HasSamples() && head.Changes() != 0 {
			t.Errorf("%s: a head read holds samples", file)
		}

		whole, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		full, err := Read(whole)
		if err != nil {
			continue
		}

		switch {
		case head.FPS != full.FPS || head.Frames != full.Frames:
			t.Errorf("%s: the head says %g fps over %d frames, the whole file %g over %d",
				file, head.FPS, head.Frames, full.FPS, full.Frames)
		case len(head.Joints) != len(full.Joints):
			t.Errorf("%s: the head holds %d joints, the whole file %d", file, len(head.Joints), len(full.Joints))
		case head.ScaleWidth != full.ScaleWidth:
			t.Errorf("%s: the head scales %d wide, the whole file %d", file, head.ScaleWidth, full.ScaleWidth)
		}

		for index := range head.Joints {
			if head.Joints[index].Name != full.Joints[index].Name || head.Joints[index].Changes != full.Joints[index].Changes {
				t.Errorf("%s: joint %d is %+v in the head, %+v in the whole file", file, index, head.Joints[index], full.Joints[index])

				break
			}
		}
	}

	t.Logf("read the head of %d animations from their first %d bytes; %d rigs did not fit", read, HeadBytes, cut)
}
