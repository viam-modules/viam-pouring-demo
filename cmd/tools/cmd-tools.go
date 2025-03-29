package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

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

	ms := vmodutils.AddMachineFlags()

	flag.Parse()

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

	weight, err := sensor.FromRobot(client, "scale")
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

	g := pour.NewTesting(logger, arm, gripper, cam, weight, motion, camVision)

	cmd := flag.Arg(0)
	switch cmd {
	case "reset":
		return g.ResetArmToHome(ctx)
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}

	return nil
}
