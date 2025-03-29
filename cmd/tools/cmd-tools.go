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
	if err != nil {
		logger.Warnf("no arm: %v", err)
	}

	gripper, err := gripper.FromRobot(client, "gripper")
	if err != nil {
		logger.Warnf("no gripper: %v", err)
	}

	cam, err := camera.FromRobot(client, "cam1")
	if err != nil {
		logger.Warnf("no camera: %v", err)
	}

	weight, err := sensor.FromRobot(client, "scale-hc")
	if err != nil {
		logger.Warnf("no weight: %v", err)
	}

	motion, err := motion.FromRobot(client, "builtin")
	if err != nil {
		logger.Warnf("no motion: %v", err)
	}

	camVision, err := vision.FromRobot(client, "circle-service")
	if err != nil {
		logger.Warnf("no vision service: %v", err)
	}

	g := pour.NewTesting(logger, client, arm, gripper, cam, weight, motion, camVision)

	cmd := flag.Arg(0)
	switch cmd {
	case "reset":
		return g.ResetArmToHome(ctx)
	case "pick-far":
		return g.PickFarBottle(ctx)
	case "pick-mid":
		return g.PickMiddleBottle(ctx)
	case "visWorldState":
		return visObstacles(arm)
	case "pour":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true})
	case "pour-far":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromFar: true})
	case "pour-mid":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromMid: true})
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}

	return nil
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
