package main

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func touch(ctx context.Context, myRobot robot.Robot, c *pour.Pour1Components, logger logging.Logger) error {

	touchPointRaw3d, err := findTouchPoint3d(ctx, c.CroppedCupCamera, logger)
	if err != nil {
		return err
	}

	logger.Infof("touchPointRaw3d: %v", touchPointRaw3d)

	touchPointRaw3db, err := findTouchPoint3db(ctx, c.CroppedCupCamera, logger)
	if err != nil {
		return err
	}

	logger.Infof("touchPointRaw3db: %v", touchPointRaw3db)

	panic(1)

	worldState, err := referenceframe.NewWorldState(nil, nil)

	logger.Infof("going to move")
	done, err := c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: "gripper-tip"},
			Destination:   touchPointRaw3d,
			WorldState:    worldState,
		},
	)
	if err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("first move didn't finish")
	}

	time.Sleep(time.Second * 10)

	return nil
}

func findTouchPoint3d(ctx context.Context, cam camera.Camera, logger logging.Logger) (*referenceframe.PoseInFrame, error) {
	if cam == nil {
		return nil, fmt.Errorf("no croppedcupcamera")
	}

	pc, err := cam.NextPointCloud(ctx)
	if err != nil {
		return nil, err
	}

	closest := r3.Vector{0, 0, 0}

	pc.Iterate(0, 0, func(p r3.Vector, d pointcloud.Data) bool {
		if closest.Z == 0 || p.Z > closest.Z {
			closest = p
		}
		return true
	})

	logger.Infof("closest in 3d cam: %v", closest)

	return referenceframe.NewPoseInFrame(
		cam.Name().ShortName(),
		spatialmath.NewPoseFromPoint(closest),
	), nil
}

func findTouchPoint3db(ctx context.Context, cam camera.Camera, logger logging.Logger) (*referenceframe.PoseInFrame, error) {
	pc, err := cam.NextPointCloud(ctx)
	if err != nil {
		return nil, err
	}

	md := pc.MetaData()

	logger.Infof("max side: %v", md.MaxSideLength())

	return nil, fmt.Errorf("finish me")

}
