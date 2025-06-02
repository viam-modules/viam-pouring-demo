package main

import (
	"context"
	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/switch"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"

	"github.com/erh/vmodutils/touch"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func planperf(ctx context.Context, myRobot robot.Robot, cfg *pour.Config, p1c *pour.Pour1Components, vc *pour.VinoCart, logger logging.Logger) error {
	fs, err := touch.FrameSystemWithSomeParts(ctx, myRobot, []string{"arm-right", "gripper-right"}, nil)
	if err != nil {
		return err
	}

	logger.Infof("fs: %v", fs)

	startJoints := []referenceframe.Input{
		{-1.6046726703643799},
		{-0.9392223954200745},
		{-0.28884029388427734},
		{4.769320487976074},
		{1.0797568559646606},
		{-2.8038926124572754},
	}

	dest := referenceframe.NewPoseInFrame("world", spatialmath.NewPose(r3.Vector{X: 191.391061, Y: 297.871836, Z: 371.730225},
		&spatialmath.OrientationVectorDegrees{OX: 0.801501, OY: -0.597993, OZ: -0.000224, Theta: 101.891328}))

	start := time.Now()

	planReq := &motionplan.PlanRequest{
		Logger:      logger,
		FrameSystem: fs,
		Goals: []*motionplan.PlanState{
			motionplan.NewPlanState(referenceframe.FrameSystemPoses{"gripper-right": dest}, nil),
		},
		StartState: motionplan.NewPlanState(nil, referenceframe.FrameSystemInputs{"arm-right": startJoints}),
	}

	plan, err := motionplan.PlanMotion(ctx, planReq)
	if err != nil {
		return err
	}

	logger.Infof("took %v to plan", time.Since(start))
	logger.Infof("plan: %v", plan)

	return nil
}

func plan(ctx context.Context, myRobot robot.Robot, cfg *pour.Config, p1c *pour.Pour1Components, vc *pour.VinoCart, logger logging.Logger) error {

	if true {
		start := time.Now()
		_, _, err := vc.DebugGetGlassPourCamImage(ctx, 5)
		if err != nil {
			return err
		}
		logger.Infof("time to get image %v", time.Since(start))
	}

	if true {
		s, err := toggleswitch.FromRobot(myRobot, "arm-pour-right-prep")
		if err != nil {
			return err
		}
		err = s.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}

		s, err = toggleswitch.FromRobot(myRobot, "arm-pour-right-pos0")
		if err != nil {
			return err
		}
		err = s.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}
	}

	bottleName := "bottle-top"

	bottleTop := referenceframe.NewLinkInFrame(
		cfg.BottleGripper,
		spatialmath.NewPose(r3.Vector{cfg.BottleHeight - 70, -7, 0}, &spatialmath.OrientationVectorDegrees{OX: 1}),
		bottleName,
		nil,
	)

	extraFrames := []*referenceframe.LinkInFrame{bottleTop}

	pif, err := p1c.BottleMotionService.GetPose(ctx, gripper.Named(bottleName), "world", extraFrames, nil)
	if err != nil {
		return err
	}

	logger.Infof("bottleTop: %v", pif.Pose())

	worldState, err := referenceframe.NewWorldState(nil, extraFrames)
	if err != nil {
		return err
	}

	o := pif.Pose().Orientation().OrientationVectorDegrees()

	for i := 0; i < 20; i++ {
		o.OZ -= .05

		goalPose := referenceframe.NewPoseInFrame("world",
			spatialmath.NewPose(
				pif.Pose().Point(),
				o,
			),
		)

		logger.Infof("going to: %v", goalPose.Pose())

		_, err = p1c.BottleMotionService.Move(
			ctx,
			motion.MoveReq{
				ComponentName: resource.Name{Name: bottleName},
				Destination:   goalPose,
				WorldState:    worldState,
			},
		)
		if err != nil {
			return err
		}
	}
	return err
}
