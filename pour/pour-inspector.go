package pour

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/utils"
)

type TrainingLabel string

const goodPour = TrainingLabel("good-pour")
const underPour = TrainingLabel("under-pour")
const overPour = TrainingLabel("over-pour")
const pourQualityDatasetID = "69ebb260ec6ccb3ae5466448"
const trainingDataDirName = "training"

func dirnameForPour(pour time.Time) string {
	return filepath.Join(trainingDataDirName, pour.Format("20060102_150405.000"))
}

// Inspects and trains pours.
// Use to detect if a pour is considered "good" via CV,
// or to label pours as good, over-, or under-poured
// to train the vision model.
type pourInsepctor struct {
	visionService vision.Service
	dataClient    *app.DataClient
	cameraName    *resource.Name
	logger        logging.Logger
}

// Check if the pour is considered good using computer vision
func (pi *pourInsepctor) checkGoodPour(ctx context.Context, img image.Image) bool {
	if pi.visionService == nil {
		return false
	}
	classifications, err := pi.visionService.Classifications(ctx, img, 1, nil)
	if err != nil {
		return false
	}
	if len(classifications) == 0 {
		return false
	}

	classification := classifications[0]
	label := classification.Label()
	score := classification.Score()
	if label == string(goodPour) && score >= 80 {
		return true
	}
	return false
}

// Label the images from the provided pour with the given label.
//
//   - If the label is an `under-pour`, then the most last two images of the pour will be labaled as such.
//   - If the label is a `good-pour`, then the last image will be labled a `good-pour`, and the two images prior will be labeled `under-pour`
//   - If the label is an `over-pour`, then the last image will be labeled an `over-pour`,
//     the two images prior will be labeled `good-pour`,
//     and the two images further prior will be labeled `under-pour`
func (pi *pourInsepctor) labelPour(ctx context.Context, pour time.Time, label TrainingLabel) error {
	folderName := dirnameForPour(pour)
	defer func() {
		if err := pi.cleanupImages(folderName); err != nil {
			pi.logger.Errorf("failed to cleanup images: %v", err)
		}
	}()

	if age := time.Since(pour); age > 2*time.Minute {
		pi.logger.Infof("skipping upload of stale images (age: %v): %s", age, folderName)
		return nil
	}

	return pi.uploadTaggedImages(ctx, folderName, label)
}

func findFiles(ctx context.Context, root string) ([]string, error) {
	var files []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (pi *pourInsepctor) cleanupImages(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return nil
}

func (pi *pourInsepctor) uploadTaggedImages(ctx context.Context, folderPath string, label TrainingLabel) error {
	pid := os.Getenv(utils.MachinePartIDEnvVar)
	if pid == "" {
		return fmt.Errorf("%s not defined", utils.MachinePartIDEnvVar)
	}
	pourTime := filepath.Base(folderPath)
	notFullOpts := &app.FileUploadOptions{
		ComponentName: &pi.cameraName.Name,
		Tags:          []string{"not-full", pourTime},
		DatasetIDs:    []string{pourQualityDatasetID},
	}

	fullOpts := &app.FileUploadOptions{
		ComponentName: &pi.cameraName.Name,
		Tags:          []string{"full", pourTime},
		DatasetIDs:    []string{pourQualityDatasetID},
	}

	files, err := findFiles(ctx, folderPath)
	if err != nil {
		return err
	}

	pi.logger.Infof("found %d files in folder %s", len(files), folderPath)

	numFiles := len(files)

	// Our logic uploads a max of 2 not-full images per pour
	// This is due to how during a pour, the vast majority of images are
	// in a not-full state. Once the full state is hit, we stop pouring.
	// If we were to upload all images, then they'd be heavily skewed towards not-full
	switch label {

	// tag last 2 images as not full
	case underPour:
		// guard against index out of bounds
		endBoundary := max(0, numFiles-2)

		for _, filepath := range files[endBoundary:] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, filepath, notFullOpts); err != nil {
				return err
			}
		}

	// tag the last 2 images before the stopping point as not-full
	case goodPour:
		// guard against index out of bounds
		threeFromEndBoundary := max(0, numFiles-3)
		oneFromEndBoundary := max(0, numFiles-1)

		for _, path := range files[threeFromEndBoundary:oneFromEndBoundary] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, notFullOpts); err != nil {
				return err
			}
		}
		// tag the last image as full
		if numFiles > 0 {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, files[numFiles-1], fullOpts); err != nil {
				return err
			}
		}

	// We assume that the last 3 images are full, this is an approximation of the amount of over-pouring
	// We also label the 2 images before the full/over-pour section as not-full
	case overPour:
		// guard against index out of bounds
		fiveFromEndBoundary := max(0, numFiles-5)
		threeFromEndBoundary := max(0, numFiles-3)

		for _, path := range files[fiveFromEndBoundary:threeFromEndBoundary] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, notFullOpts); err != nil {
				return err
			}
		}

		for _, path := range files[threeFromEndBoundary:] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, fullOpts); err != nil {
				return err
			}
		}
	default:
		return errors.New("not valid label")
	}
	return nil
}
