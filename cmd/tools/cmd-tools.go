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

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
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
		return vc.PourMotionDemo(ctx)
	case "sleep":
		time.Sleep(time.Minute * 5)
		return nil
	case "loop":
		return vc.Loop(ctx)

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
