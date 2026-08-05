package pour

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type TrainingLabel string

const trainingDataDirName = "training"

func dirnameForPour(pour time.Time) string {
	return filepath.Join(trainingDataDirName, pour.Format("20060102_150405.000"))
}

func findFiles(root string) ([]string, error) {
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

func cleanupImages(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return nil
}
