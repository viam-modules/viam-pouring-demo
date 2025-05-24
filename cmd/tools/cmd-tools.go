package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	vizClient "github.com/viam-labs/motion-tools/client/client"
	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"

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

	flag.BoolVar(&debug, "debug", false, "")
	host := flag.String("host", "", "host to connect to")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	if flag.NArg() == 0 {
		return fmt.Errorf("need a config file")
	}

	cfg := &pour.Config{}
	err := vmodutils.ReadJSONFromFile(flag.Arg(0), cfg)
	if err != nil {
		return err
	}

	client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	arm, err := arm.FromRobot(client, "arm")
	if err != nil || arm == nil {
		logger.Fatalf("no arm: %v", err)
	}
	j, err := arm.JointPositions(ctx, nil)
	if err != nil {
		logger.Fatalf("arm erroring: %v", err)
	}
	logger.Infof("arm current positions %v", j)

	gripper, err := gripper.FromRobot(client, "gripper")
	if err != nil || gripper == nil {
		logger.Fatalf("no gripper: %v", err)
	}

	cam, err := camera.FromRobot(client, "cam1")
	if err != nil || cam == nil {
		logger.Fatalf("no camera: %v", err)
	}

	weight, err := sensor.FromRobot(client, "scale1")
	if err != nil || weight == nil {
		logger.Fatalf("no weight: %v", err)
	}

	motion, err := motion.FromRobot(client, "builtin")
	if err != nil || motion == nil {
		logger.Fatalf("no motion: %v", err)
	}

	camVision, err := vision.FromRobot(client, "circle-service")
	if err != nil || camVision == nil {
		logger.Fatalf("no vision service: %v", err)
	}

	g := pour.NewTesting(logger, client, arm, gripper, cam, weight, motion, camVision)

	cmd := flag.Arg(0)
	switch cmd {
	case "reset":
		return g.ResetArmToHome(ctx)
	case "intermediate":
		return g.GoToPrepForPour(ctx)
	case "touch-prep":
		return touchPrep(ctx, client, motion, arm, cam, logger)
	case "touch":
		return touch(ctx, client, motion, arm, cam, logger)
	case "print-world":
		printPoseInfo(ctx, motion, cam.Name(), logger)
		printPoseInfo(ctx, motion, arm.Name(), logger)
		printPoseInfo(ctx, motion, gripper.Name(), logger)
		return nil
	case "visWorldState":
		return visObstacles(ctx, client)
	case "plan":
		return g.StartPouringProcess(ctx, pour.PouringOptions{})
	case "pour":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true})
	case "pour-far":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromFar: true})
	case "pour-mid":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromMid: true})
	case "find-cups":
		cups, err := g.FindCups(ctx)
		if err != nil {
			return err
		}
		for idx, c := range cups {
			logger.Infof("cup %d : %v", idx, c)
		}
		return nil
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}

func visObstacles(ctx context.Context, myRobot robot.Robot) error {

	err := vizClient.RemoveAllSpatialObjects()
	if err != nil {
		return err
	}

	err = vizClient.DrawRobot(ctx, myRobot, nil)
	if err != nil {
		return err
	}

	for _, g := range pour.GenerateObstacles() {
		for _, actualGeom := range g.Geometries() {
			err = vizClient.DrawGeometry(actualGeom, "red")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func printPoseInfo(ctx context.Context, motion motion.Service, name resource.Name, logger logging.Logger) {
	p, err := motion.GetPose(ctx, name, referenceframe.World, nil, nil)
	if err != nil {
		logger.Warnf("cannot get pose for %v : %v", err)
	} else {
		fmt.Printf("%v: %v %T\n", name, p.Pose(), p.Pose())
	}
}
