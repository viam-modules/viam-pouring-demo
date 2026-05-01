package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/robot"
)

type captureDirectOptions struct {
	outDir      string
	gripperName string
	camNames    []string
	instruction string
	hz          int
}

type poseSample struct {
	t  time.Time
	ee []float64
}

type camSample struct {
	t      time.Time
	rel    string
	camIdx int
}

func runCaptureDirect(ctx context.Context, client robot.Robot, opts captureDirectOptions, logger logging.Logger) error {
	if opts.hz <= 0 {
		return fmt.Errorf("-hz must be positive, got %d", opts.hz)
	}

	var camComps []camera.Camera
	for _, name := range opts.camNames {
		c, err := camera.FromRobot(client, name)
		if err != nil {
			return fmt.Errorf("camera %q: %w", name, err)
		}
		camComps = append(camComps, c)
	}

	sessionID := time.Now().UTC().Format("20060102T150405.000Z")
	epDir := filepath.Join(opts.outDir, sessionID)
	for i := range camComps {
		imgDir := filepath.Join(epDir, fmt.Sprintf("images_%d", i))
		if err := os.MkdirAll(imgDir, 0o755); err != nil {
			return err
		}
	}

	period := time.Second / time.Duration(opts.hz)
	captureCtx, stopCapture := context.WithCancel(ctx)
	var wg sync.WaitGroup
	var (
		poseSamples []poseSample
		camSamples  []camSample
		muPose      sync.Mutex
		muCam       sync.Mutex
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-ticker.C:
				t := time.Now()
				worldPIF, perr := client.GetPose(ctx, opts.gripperName, referenceframe.World, nil, nil)
				if perr != nil {
					logger.Warnf("framesystem GetPose err: %v", perr)
					continue
				}
				worldPose := worldPIF.Pose()
				ovd := worldPose.Orientation().OrientationVectorDegrees()
				ee := []float64{
					worldPose.Point().X, worldPose.Point().Y, worldPose.Point().Z,
					ovd.OX, ovd.OY, ovd.OZ, ovd.Theta,
				}
				muPose.Lock()
				poseSamples = append(poseSamples, poseSample{t: t, ee: ee})
				muPose.Unlock()
			}
		}
	}()

	for camIdx, camComp := range camComps {
		wg.Add(1)
		go func(camIdx int, camComp camera.Camera) {
			defer wg.Done()
			ticker := time.NewTicker(period)
			defer ticker.Stop()
			idx := 0
			for {
				select {
				case <-captureCtx.Done():
					return
				case <-ticker.C:
					t := time.Now()
					imgs, _, err := camComp.Images(ctx, nil, nil)
					if err != nil || len(imgs) == 0 {
						logger.Debugf("cam[%d] read err: %v (n=%d)", camIdx, err, len(imgs))
						continue
					}
					bs, err := imgs[0].Bytes(ctx)
					if err != nil {
						logger.Warnf("cam[%d] bytes err: %v", camIdx, err)
						continue
					}
					ext := mimeExt(imgs[0].MimeType())
					rel := filepath.Join(fmt.Sprintf("images_%d", camIdx), fmt.Sprintf("step_%06d%s", idx, ext))
					if err := os.WriteFile(filepath.Join(epDir, rel), bs, 0o644); err != nil {
						logger.Debugf("write img: %v", err)
						continue
					}
					idx++
					muCam.Lock()
					camSamples = append(camSamples, camSample{t: t, rel: rel, camIdx: camIdx})
					muCam.Unlock()
				}
			}
		}(camIdx, camComp)
	}

	logger.Infof("capture-direct: started gripper-pose + cam goroutines @ %dHz, episode dir %s", opts.hz, epDir)
	fmt.Println("Recording... press Enter to stop.")
	fmt.Scanln()
	stopCapture()
	wg.Wait()

	logger.Infof("captured %d pose samples, %d cam samples", len(poseSamples), len(camSamples))
	if err := writeOpenVLAEpisode(epDir, camSamples, len(camComps), poseSamples, opts.instruction); err != nil {
		return fmt.Errorf("writing episode: %w", err)
	}
	return nil
}

func writeOpenVLAEpisode(epDir string, cam []camSample, numCams int, poses []poseSample, instruction string) error {
	// Split samples by camera index. Camera 0 is the primary timeline.
	perCam := make([][]camSample, numCams)
	for _, c := range cam {
		perCam[c.camIdx] = append(perCam[c.camIdx], c)
	}
	primary := perCam[0]
	if len(primary) == 0 {
		return fmt.Errorf("no camera samples from primary camera")
	}

	f, err := os.Create(filepath.Join(epDir, "steps.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)

	for i, c := range primary {
		var ee []float64
		if p := closestPose(poses, c.t); p != nil {
			ee = p.ee
		}

		step := map[string]any{
			"step_index":           i,
			"timestamp":            c.t.UTC().Format(time.RFC3339Nano),
			"is_first":             i == 0,
			"is_last":              i == len(primary)-1,
			"is_terminal":          i == len(primary)-1,
			"image":                c.rel,
			"ee_pose":              ee,
			"language_instruction": instruction,
		}

		for camIdx := 1; camIdx < numCams; camIdx++ {
			key := fmt.Sprintf("image_%d", camIdx)
			if closest := closestCam(perCam[camIdx], c.t); closest != nil {
				step[key] = closest.rel
			}
		}

		if err := enc.Encode(step); err != nil {
			return err
		}
	}
	return nil
}

func closestCam(rows []camSample, t time.Time) *camSample {
	if len(rows) == 0 {
		return nil
	}
	best := &rows[0]
	bestD := absDur(rows[0].t.Sub(t))
	for i := 1; i < len(rows); i++ {
		d := absDur(rows[i].t.Sub(t))
		if d < bestD {
			best = &rows[i]
			bestD = d
		}
	}
	return best
}

func closestPose(rows []poseSample, t time.Time) *poseSample {
	if len(rows) == 0 {
		return nil
	}
	best := &rows[0]
	bestD := absDur(rows[0].t.Sub(t))
	for i := 1; i < len(rows); i++ {
		d := absDur(rows[i].t.Sub(t))
		if d < bestD {
			best = &rows[i]
			bestD = d
		}
	}
	return best
}

func mimeExt(mt string) string {
	switch mt {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	default:
		return ".bin"
	}
}
