package main

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/switch"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
	"go.viam.com/rdk/utils"
	viz "go.viam.com/rdk/vision"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func touch(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, cfg *pour.Config, logger logging.Logger) error {
	logger.Infof("touch called")
	if false {
		obstacleTable, err := gripper.FromRobot(myRobot, "obstacle-table")
		if err != nil {
			return err
		}

		g, err := obstacleTable.Geometries(ctx, nil)
		if err != nil {
			return err
		}
		logger.Infof("obstacle table: %v", g)
		return nil
	}

	err := c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	objects, err := c.CupFinder.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return err
	}

	logger.Infof("num objects: %v", len(objects))
	for _, o := range objects {
		logger.Infof("\t objects: %v", o)
	}

	if len(objects) == 0 {
		return fmt.Errorf("no objects")
	}

	if len(objects) > 1 {
		return fmt.Errorf("too many objects %d", len(objects))
	}

	if cfg.SimoneHack {
		err = c.LeftRetreat.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}

		err = c.LeftPlace.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}

		_, err = c.Gripper.Grab(ctx, nil)
		if err != nil {
			return err
		}

		return nil
	}

	obj := objects[0]

	// -- approach

	goToPose := getApproachPoint(obj, 100, 0)
	logger.Infof("going to move to %v", goToPose)

	obstacles := []*referenceframe.GeometriesInFrame{}
	obstacles = append(obstacles, referenceframe.NewGeometriesInFrame("world", []spatialmath.Geometry{obj.Geometry}))
	logger.Infof("add cup as obstacle %v", obj.Geometry)

	worldState, err := referenceframe.NewWorldState(obstacles, nil)
	if err != nil {
		return err
	}

	_, err = c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: c.Gripper.Name().ShortName()},
			Destination:   goToPose,
			WorldState:    worldState,
		},
	)
	if err != nil {
		return err
	}

	// ---- go to pick up

	goToPose = getApproachPoint(obj, -30, 0)
	logger.Infof("going to move to %v", goToPose)

	_, err = c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: c.Gripper.Name().ShortName()},
			Destination:   goToPose,
			Constraints:   &pour.LinearConstraint,
		},
	)
	if err != nil {
		return err
	}

	_, err = c.Gripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func getApproachPoint(obj *viz.Object, deltaX, deltaZ float64) *referenceframe.PoseInFrame {
	md := obj.MetaData()
	c := md.Center()

	approachPoint := r3.Vector{
		Y: c.Y,
		Z: 95 + deltaZ,
	}

	if md.MinX > 0 {
		approachPoint.X = md.MinX - deltaX
	} else {
		approachPoint.X = md.MaxX + deltaX
	}

	return referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(
			approachPoint,
			&spatialmath.OrientationVectorDegrees{OX: 1, Theta: 180}),
	)

}

func doAll(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger, all []toggleswitch.Switch) error {
	for _, s := range all {
		err := s.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func pourPrepGrab(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger) error {

	positions, err := c.BottleArm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	orig := positions[0]

	logger.Infof("pourPrepGrab orig: %v", orig)
	positions[0].Value -= utils.DegToRad(2)
	logger.Infof("pourPrepGrab hack: %v", positions[0])

	err = c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)

	_, err = c.BottleGripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)

	positions[0] = orig
	err = c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	return nil
}

func pourPrep(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger) error {
	err := doAll(ctx, myRobot, c, logger, c.RightBottlePourPreGrabActions)
	if err != nil {
		return err
	}

	err = pourPrepGrab(ctx, myRobot, c, logger)
	if err != nil {
		return err
	}

	err = doAll(ctx, myRobot, c, logger, c.RightBottlePourPostGrabActions)
	if err != nil {
		return err
	}

	return nil
}

func pourNew(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger) error {
	positions, err := c.BottleArm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	positionsLeft, err := c.Arm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	err = pour.SetXarmSpeed(ctx, c.BottleArm, 20, 100) // slow down
	if err != nil {
		return err
	}

	orig := positions[5]

	positions[5].Value = utils.DegToRad(-170)

	err = c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	positions[5] = orig

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = c.BottleArm.MoveToJointPositions(ctx, positions, nil)
		logger.Errorf("error tilting bottle: %v", err)
	}()

	{
		err = pour.SetXarmSpeed(ctx, c.Arm, 20, 100) // back to default
		if err != nil {
			return err
		}

		positionsLeft[5].Value -= utils.DegToRad(-15)
		err = c.Arm.MoveToJointPositions(ctx, positionsLeft, nil)
		if err != nil {
			return err
		}

		err = pour.SetXarmSpeed(ctx, c.Arm, 60, 100) // back to default
		if err != nil {
			return err
		}

	}

	wg.Wait()

	err = pour.SetXarmSpeed(ctx, c.BottleArm, 60, 100) // back to default
	if err != nil {
		return err
	}

	return nil
}

func putBack(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger) error {
	x := append([]toggleswitch.Switch{}, c.RightBottlePourPreGrabActions...)
	slices.Reverse(x)

	err := x[0].SetPosition(ctx, 2, nil)
	if err != nil {
		return err
	}

	err = c.BottleGripper.Open(ctx, nil)
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 500)

	err = doAll(ctx, myRobot, c, logger, x)

	err = c.LeftPlace.SetPosition(ctx, 2, nil)
	if err != nil {
		return err
	}

	err = c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	err = c.LeftRetreat.SetPosition(ctx, 2, nil)
	if err != nil {
		return err
	}

	// just to get arms back to home
	_, err = c.CupFinder.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return err
	}

	return nil
}
