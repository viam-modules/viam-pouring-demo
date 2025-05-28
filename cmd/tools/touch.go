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
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/spatialmath"
	viz "go.viam.com/rdk/vision"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

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

	cupFinderService, err := vision.FromRobot(myRobot, "cup-finder")
	if err != nil {
		return err
	}

	objects, err := cupFinderService.GetObjectPointClouds(ctx, "", nil)
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

	approachPose := getApproachPoint(obj)

	logger.Infof("going to move to %v", approachPose)

	obstacles := []*referenceframe.GeometriesInFrame{}
	obstacles = append(obstacles, referenceframe.NewGeometriesInFrame("world", []spatialmath.Geometry{obj.Geometry}))
	worldState, err := referenceframe.NewWorldState(obstacles, nil)

	if err != nil {
		return err
	}

	done, err := c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: "gripper-tip"},
			Destination:   approachPose,
			WorldState:    worldState,
		},
	)
	if err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("first move didn't finish")
	}

	return nil
}

func getApproachPoint(obj *viz.Object) *referenceframe.PoseInFrame {
	md := obj.MetaData()
	c := md.Center()

	return referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(
			r3.Vector{
				X: md.MaxX + 50,
				Y: c.Y,
				Z: 200,
			},
			&spatialmath.OrientationVectorDegrees{OX: -1, Theta: 180}),
	)

}
