package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	vizClient "github.com/viam-labs/motion-tools/client"
	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
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
	ms := vmodutils.AddMachineFlags()

	flag.BoolVar(&debug, "debug", false, "")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	client, err := ms.Connect(ctx, logger)
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
	logger.Infof("current positions", j)

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
	case "visWorldState":
		return visObstacles(arm)
	case "plan":
		return g.StartPouringProcess(ctx, pour.PouringOptions{})
	case "pour":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true})
	case "pour-far":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromFar: true})
	case "pour-mid":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromMid: true})
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}

func visObstacles(arm arm.Arm) error {

	armGeoms, err := arm.Geometries(context.Background(), nil)
	if err != nil {
		return err
	}

	for _, g := range armGeoms {
		vizClient.DrawGeometry(g, "blue")
	}

	gifs := pour.GenerateObstacles()

	for _, g := range gifs {
		for _, actualGeom := range g.Geometries() {
			vizClient.DrawGeometry(actualGeom, "red")
		}
	}

	return nil
}
