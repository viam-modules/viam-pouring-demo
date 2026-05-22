package pour

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/utils"
)

// Dataset to upload/train pour quality on
const pourQualityDatasetID = "69ebb260ec6ccb3ae5466448"

// Training label indicating a good pour
const goodPour = TrainingLabel("good-pour")

// Training label indicating an under pour
const underPour = TrainingLabel("under-pour")

// Training label indicating an over pour
const overPour = TrainingLabel("over-pour")

// The point after which images are ignored
const staleTime = 2 * time.Minute

// The score of the classification to be considered "accurate"
const classificationThreshold = 0.80

// Inspects and trains pours.
// Use to detect if a pour is considered "good" via CV,
// or to label pours as good, over-, or under-poured
// to train the vision model.
type pourInsepctor struct {
	visionService vision.Service
	dataClient    *app.DataClient
	cameraName    resource.Name
	logger        logging.Logger
}

// Check if the pour is considered good using computer vision
func (pi *pourInsepctor) checkGoodPour(ctx context.Context, img image.Image) (bool, error) {
	if pi.visionService == nil {
		return false, errors.New("no vision service provided")
	}
	classifications, err := pi.visionService.Classifications(ctx, img, 1, nil)
	if err != nil {
		return false, err
	}
	if len(classifications) == 0 {
		pi.logger.Debug("[PI] Classifications length is zero")
		return false, nil
	}

	classification := classifications[0]
	label := classification.Label()
	score := classification.Score()
	pi.logger.Debugf("[PI] Pour classification: %s - %0.2f", label, score)

	// We want to stop on a good pour or over pour
	if (label == string(goodPour) || label == string(overPour)) && score >= classificationThreshold {
		return true, nil
	}

	return false, nil
}

// Label the images from the provided pour with the given label.
//
//   - If the label is an `under-pour`, then the most last two images of the pour will be labaled as such.
//   - If the label is a `good-pour`, then the last image will be labled a `good-pour`, and the two images prior will be labeled `under-pour`
//   - If the label is an `over-pour`, then the last two images will be labeled an `over-pour`,
//     the two images prior will be labeled `good-pour`,
//     and the two images further prior will be labeled `under-pour`
func (pi *pourInsepctor) labelPour(ctx context.Context, pour time.Time, label TrainingLabel) error {
	folderName := dirnameForPour(pour)
	defer func() {
		if err := cleanupImages(folderName); err != nil {
			pi.logger.Errorf("failed to cleanup images: %v", err)
		}
	}()

	if age := time.Since(pour); age > staleTime {
		pi.logger.Infof("skipping upload of stale images (age: %v): %s", age, folderName)
		return nil
	}

	return pi.uploadTaggedImages(ctx, folderName, label)
}

func (pi *pourInsepctor) uploadTaggedImages(ctx context.Context, folderPath string, label TrainingLabel) error {
	pid := os.Getenv(utils.MachinePartIDEnvVar)
	if pid == "" {
		return fmt.Errorf("%s not defined", utils.MachinePartIDEnvVar)
	}
	pourTime := filepath.Base(folderPath)

	underPourOpts := &app.FileUploadOptions{
		ComponentName: &pi.cameraName.Name,
		Tags:          []string{string(underPour), pourTime},
		DatasetIDs:    []string{pourQualityDatasetID},
	}
	goodPourOpts := &app.FileUploadOptions{
		ComponentName: &pi.cameraName.Name,
		Tags:          []string{string(goodPour), pourTime},
		DatasetIDs:    []string{pourQualityDatasetID},
	}
	overPourOpts := &app.FileUploadOptions{
		ComponentName: &pi.cameraName.Name,
		Tags:          []string{string(overPour), pourTime},
		DatasetIDs:    []string{pourQualityDatasetID},
	}

	files, err := findFiles(folderPath)
	if err != nil {
		return err
	}

	numFiles := len(files)

	pi.logger.Infof("found %d files in folder %s", numFiles, folderPath)

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
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, filepath, underPourOpts); err != nil {
				return err
			}
		}

	// tag the last 2 images before the stopping point as not-full
	case goodPour:
		// guard against index out of bounds
		threeFromEndBoundary := max(0, numFiles-3)
		oneFromEndBoundary := max(0, numFiles-1)

		for _, path := range files[threeFromEndBoundary:oneFromEndBoundary] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, underPourOpts); err != nil {
				return err
			}
		}
		// tag the last image as full
		if numFiles > 0 {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, files[numFiles-1], goodPourOpts); err != nil {
				return err
			}
		}

	// We assume that the last 2 images are over-pour,
	// the two before that as good-pour,
	// and the two before that as under-pour.
	//
	// This is an approximation of the amount of over-pouring
	case overPour:
		// guard against index out of bounds
		sixFromEndBoundary := max(0, numFiles-6)
		fourFromEndBoundary := max(0, numFiles-4)
		twoFromEndBoundary := max(0, numFiles-2)

		for _, path := range files[sixFromEndBoundary:fourFromEndBoundary] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, underPourOpts); err != nil {
				return err
			}
		}

		for _, path := range files[fourFromEndBoundary:twoFromEndBoundary] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, goodPourOpts); err != nil {
				return err
			}
		}

		for _, path := range files[twoFromEndBoundary:] {
			if _, err := pi.dataClient.FileUploadFromPath(ctx, pid, path, overPourOpts); err != nil {
				return err
			}
		}
	default:
		return errors.New("not valid label")
	}
	return nil
}
