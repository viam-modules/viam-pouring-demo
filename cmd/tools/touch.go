package main

import (
	"context"
	"fmt"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
	viz "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/viscapture"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

var extra = map[string]interface{}{"num_threads": 50}

func touch(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger) error {
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

	objects, err := c.CupFinder.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return err
	}

	err = c.Gripper.Open(ctx, nil)
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
			Extra:         extra,
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
			Extra:         extra,
		},
	)
	if err != nil {
		return err
	}

	_, err = c.Gripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	goToPose = getApproachPoint(obj, -30, 100)
	logger.Infof("going to move to %v", goToPose)

	_, err = c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: c.Gripper.Name().ShortName()},
			Destination:   goToPose,
			Extra:         extra,
		},
	)
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

func alignCup(ctx context.Context, myRobot robot.Robot, cfg *pour.Config, c *pour.Pour1Components, logger logging.Logger) error {
	if false {
		res, err := c.CupTop.CaptureAllFromCamera(ctx, cfg.CameraName, viscapture.CaptureOptions{ReturnImage: true, ReturnDetections: true}, nil)
		if err != nil {
			return nil
		}
		logger.Infof("res: %v", res)

		if len(res.Detections) == 0 {
			return fmt.Errorf("no detections for alignCup")
		}

		if len(res.Detections) > 1 {
			return fmt.Errorf("too many detections for alignCup %d", len(res.Detections))
		}
	}
	p, err := c.Motion.GetPose(ctx, c.Gripper.Name(), "world", nil, nil)
	if err != nil {
		return err
	}

	logger.Infof("start: %v", p)
	p = p.Transform(referenceframe.NewPoseInFrame("world", spatialmath.NewPoseFromPoint(r3.Vector{X: -100}))).(*referenceframe.PoseInFrame)
	logger.Infof("go to: %v", p)

	_, err = c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: c.Gripper.Name().ShortName()},
			Destination:   p,
			WorldState:    nil,
		},
	)
	if err != nil {
		return err
	}

	return fmt.Errorf("finish me")
}
