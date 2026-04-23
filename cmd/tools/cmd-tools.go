package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/golang/geo/r3"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/components/posetracker"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/robot"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func getPlans(ctx context.Context, logger logging.Logger, partID, startStr, endStr string) error {
	viamClient, err := app.ConnectFromCLIToken(ctx, logger)
	if err != nil {
		return fmt.Errorf("not logged in to Viam CLI (run `viam login`): %w", err)
	}
	defer viamClient.Close()
	dataClient := viamClient.DataClient()

	filter := &app.Filter{
		PartID: partID,
		TagsFilter: app.TagsFilter{
			Type: app.TagsFilterTypeMatchByOr,
			Tags: []string{"plan-file"},
		},
	}

	if startStr != "" || endStr != "" {
		var interval app.CaptureInterval
		if startStr != "" {
			t, err := time.Parse(time.RFC3339, startStr)
			if err != nil {
				return fmt.Errorf("invalid --start %q: %w", startStr, err)
			}
			interval.Start = t
		}
		if endStr != "" {
			t, err := time.Parse(time.RFC3339, endStr)
			if err != nil {
				return fmt.Errorf("invalid --end %q: %w", endStr, err)
			}
			interval.End = t
		}
		filter.Interval = interval
	}

	// Phase 1: page through metadata only (includeBinary=false allows Limit>1).
	var last string
	var allIDs []string
	var metaByID = map[string]*app.BinaryMetadata{}
	for {
		resp, err := dataClient.BinaryDataByFilter(ctx, false, &app.DataByFilterOptions{
			Filter: filter,
			Limit:  50,
			Last:   last,
		})
		if err != nil {
			return fmt.Errorf("querying data management: %w", err)
		}
		for _, d := range resp.BinaryData {
			allIDs = append(allIDs, d.Metadata.BinaryDataID)
			metaByID[d.Metadata.BinaryDataID] = d.Metadata
		}
		if len(resp.BinaryData) == 0 || resp.Last == "" {
			break
		}
		last = resp.Last
	}

	// Phase 2: fetch binary content by ID.
	total := 0
	const batchSize = 10
	for i := 0; i < len(allIDs); i += batchSize {
		end := i + batchSize
		if end > len(allIDs) {
			end = len(allIDs)
		}
		batch := allIDs[i:end]
		binaries, err := dataClient.BinaryDataByIDs(ctx, batch)
		if err != nil {
			return fmt.Errorf("fetching binary data: %w", err)
		}
		for _, d := range binaries {
			meta := metaByID[d.Metadata.BinaryDataID]
			fname := meta.FileName
			if fname == "" {
				fname = fmt.Sprintf("plan-%s.json", meta.BinaryDataID)
			}
			if err := os.WriteFile(fname, d.Binary, 0o600); err != nil {
				logger.Errorf("failed to write %s: %v", fname, err)
				continue
			}
			logger.Infof("saved %s (captured: %s, tags: %v)",
				fname,
				meta.TimeRequested.Format(time.RFC3339),
				meta.CaptureMetadata.Tags,
			)
			total++
		}
	}
	logger.Infof("downloaded %d plan file(s)", total)
	return nil
}

func main() {
	err := realMain()
	if err != nil {
		panic(err)
	}
}

func realMain() error {
	ctx := context.Background()
	logger := logging.NewLogger("cup")

	debug := false
	n := 1

	flag.BoolVar(&debug, "debug", false, "")
	flag.IntVar(&n, "n", n, "number of times to run")
	host := flag.String("host", "", "host to connect to")
	configFile := flag.String("config", "", "host to connect to")
	partID := flag.String("part-id", "", "machine part ID to query plan files from data management")
	startStr := flag.String("start", "", "start time filter for get-plans (RFC3339, e.g. 2026-04-23T10:00:00Z)")
	endStr := flag.String("end", "", "end time filter for get-plans (RFC3339)")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	// get-plans only needs a cloud client, not a robot connection
	if flag.Arg(0) == "get-plans" {
		if *partID == "" {
			return fmt.Errorf("--part-id is required for get-plans")
		}
		return getPlans(ctx, logger, *partID, *startStr, *endStr)
	}

	if *configFile == "" {
		return fmt.Errorf("need a config file")
	}

	cfg := &pour.Config{}
	err := vmodutils.ReadJSONFromFile(*configFile, cfg)
	if err != nil {
		return err
	}

	_, _, err = cfg.Validate("")
	if err != nil {
		return err
	}

	client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	deps, err := vmodutils.MachineToDependencies(client)
	if err != nil {
		return err
	}

	p1c, err := pour.Pour1ComponentsFromDependencies(cfg, deps)
	if err != nil {
		return err
	}

	var dataClient *app.DataClient
	appClient, err := app.CreateViamClientFromEnvVars(ctx, nil, logger)
	if err != nil {
		logger.Warnf("can't connect to app: %v", err)
	} else {
		defer appClient.Close()
		dataClient = appClient.DataClient()
	}

	vc, err := pour.NewVinoCart(ctx, cfg, p1c, client, dataClient, logger)
	if err != nil {
		return err
	}

	cmd := flag.Arg(0)
	switch cmd {
	case "reset":
		return vc.Reset(ctx)
	case "touch":
		return vc.Touch(ctx)
	case "pour-prep":
		return vc.PourPrep(ctx)
	case "touch-and-prep":
		err := vc.Touch(ctx)
		if err != nil {
			return err
		}
		return vc.PourPrep(ctx)
	case "touch-and-reset":
		for i := 0; i < n; i++ {
			err := vc.Touch(ctx)
			if err != nil {
				logger.Infof("error touching, continuing: %v", err)
				continue
			}
			time.Sleep(5 * time.Second)
			err = p1c.Gripper.Open(ctx, nil)
			if err != nil {
				return err
			}

			err = pour.Jog(ctx, p1c.Motion, p1c.Arm.Name(), r3.Vector{Z: 200})
			if err != nil {
				return err
			}
			err = vc.Reset(ctx)
			if err != nil {
				return err
			}
		}
		return err
	case "pour":
		return vc.Pour(ctx)
	case "put-back":
		return vc.PutBack(ctx)
	case "full-demo":
		return vc.FullDemo(ctx)
	case "full-demo-wait":
		return vc.WaitForCupAndGo(ctx)
	case "find-cups":
		cups, err := vc.FindCups(ctx)
		if err != nil {
			return err
		}
		for idx, c := range cups {
			logger.Infof("cup %d : %v", idx, c)
		}
		return nil
	case "pour-motion-demo":
		pp, err := vc.SetupPourPositions(ctx)
		if err != nil {
			return err
		}
		return vc.PourMotionDemo(ctx, pp)
	case "sleep":
		time.Sleep(time.Minute * 5)
		return nil

	case "pose":
		left, err := getAPose(ctx, client, "april-tag-tracker-left", "7")
		if err != nil {
			return err
		}

		right, err := getAPose(ctx, client, "april-tag-tracker-right", "7")
		if err != nil {
			return err
		}

		logger.Infof("left : %v", left)
		logger.Infof("right: %v", right)
		return nil
	case "pour-glass-find-crop":
		box, err := vc.PourGlassFindCroppedRect(ctx)
		if err != nil {
			return err
		}
		logger.Infof("box: %v", box)
		img, err := vc.PourGlassFindCroppedImage(ctx, box)
		if err != nil {
			return err
		}
		file, err := os.Create("foo.png")
		if err != nil {
			return fmt.Errorf("couldn't create file %v", err)
		}
		defer file.Close()
		return png.Encode(file, img)
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}

func getAPose(ctx context.Context, client robot.Robot, poseTracker, name string) (*referenceframe.PoseInFrame, error) {
	pt, err := posetracker.FromRobot(client, poseTracker)
	if err != nil {
		return nil, err
	}
	poses, err := pt.Poses(ctx, nil, nil)
	if err != nil {
		return nil, err
	}

	p, ok := poses[name]
	if !ok {
		return nil, fmt.Errorf("didn't find name [%s]", p)
	}

	return client.TransformPose(ctx, p, "world", nil)
}
