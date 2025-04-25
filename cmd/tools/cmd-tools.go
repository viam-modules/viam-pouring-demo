package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"
	apriltag "github.com/raybjork/apriltag/utils"

	vizClient "github.com/viam-labs/motion-tools/client"
	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/posetracker"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"

	"strconv"

	"github.com/golang/geo/r3"
	"github.com/viam-modules/viam-pouring-demo/pour"

	pb "go.viam.com/api/component/arm/v1"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/spatialmath"
)

var calibrationPositions = [][]float64{
	{-154.30, -44.43, -42.26, 179.72, -15.06, -10.11},
	{-153.13, -32.83, -85.74, 175.69, -57.59, -10.11},
	{-133.63, -8.30, -15.95, 201.26, 55.65, 61.38},
	{-213.00, -19.88, -85.45, 53.03, 86.73, 61.08},
	{-232.47, -1.17, -47.37, 75.29, 98.80, 61.08},
	{-85.87, 2.34, -59.41, 110.58, -74.55, 97.67},
	{-109.33, 31.92, -32.03, 55.98, -82.87, 97.66},
	{-145.17, 31.98, -71.31, 36.09, -22.39, 97.28},
}

func calibrationInputs(positionsDegs [][]float64) [][]referenceframe.Input {
	inputs := make([][]referenceframe.Input, 0)
	for _, position := range positionsDegs {
		i := referenceframe.FloatsToInputs(referenceframe.JointPositionsToRadians(&pb.JointPositions{Values: position}))
		inputs = append(inputs, i)
	}
	return inputs
}

var cameraPoseGuess = spatialmath.NewPose(
	r3.Vector{X: 82.424283, Y: -30.598170, Z: 18.752646},
	&spatialmath.OrientationVectorDegrees{OX: -0.029203, OY: -0.003337, OZ: 0.999568, Theta: -97.757338},
)

func genTagNamesUpToNum(n int) (names []string) {
	for i := range n {
		names = append(names, strconv.Itoa(i))
	}
	return
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

	poseTracker, err := posetracker.FromRobot(client, "pose_tracker")
	if err != nil || camVision == nil {
		logger.Fatalf("no pose tracker: %v", err)
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
	case "calibrate-camera":
		pose, err := apriltag.EstimateFramePose(
			ctx,
			arm,
			poseTracker,
			genTagNamesUpToNum(24),
			calibrationInputs(calibrationPositions),
			cameraPoseGuess,
			true,
		)
		if err != nil {
			return err
		}
		fmt.Printf("camera pose relative to arm: %0.6v\n", pose)
		return nil
	// NOTE(rb): commenting these out because they are not really usuable with the new camera setup
	// case "camera-calibrate-skew":
	// 	return g.CameraCalibrate(ctx, 0.295, 0.295, 0.295, 0.295)
	// case "camera-calibrate-no-skew":
	// 	return g.CameraCalibrate(ctx, 0, 0, 0, 0)
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
