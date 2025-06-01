package main

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/robot/framesystem"
	"go.viam.com/rdk/spatialmath"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func plan(ctx context.Context, myRobot robot.Robot, Cfg *pour.Config, p1c *pour.Pour1Components, logger logging.Logger) error {

	name := "arm-left"

	fs, err := frameSystemWithOnePart(ctx, myRobot, name)
	if err != nil {
		return err
	}

	start := referenceframe.FrameSystemInputs{}
	start[name] = []referenceframe.Input{
		{-4.2271599769592285},
		{0.4038045108318329},
		{-0.536893904209137},
		{4.674395561218262},
		{1.565266728401184},
		{-0.1328192502260208},
	}

	goal := motionplan.NewPlanState(
		referenceframe.FrameSystemPoses{
			name: referenceframe.NewPoseInFrame("world",
				spatialmath.NewPose(r3.Vector{X: 370, Y: 258, Z: 90}, &spatialmath.OrientationVectorDegrees{OX: 1, OY: 0, OZ: 0, Theta: -180})),
		},
		nil,
	)

	req := &motionplan.PlanRequest{
		Logger:      logger,
		FrameSystem: fs,
		Goals:       []*motionplan.PlanState{goal},
		StartState:  motionplan.NewPlanState(nil, start),
	}

	startTime := time.Now()
	plan, err := motionplan.PlanMotion(ctx, req)
	if err != nil {
		return err
	}

	logger.Infof("plan: trajectory length: %d path length: %d, planned in %v", len(plan.Trajectory()), len(plan.Path()), time.Since(startTime))
	for _, p := range plan.Path() {
		logger.Infof("\t %v", p[name])
	}

	prev := start[name]
	for _, t := range plan.Trajectory() {
		distance := referenceframe.InputsL2Distance(prev, t[name])
		logger.Infof("\t %v distance: %v", t[name], distance)
		prev = t[name]
	}

	x := []referenceframe.Input{
		{-4.654025554656983},
		{0.4523119628429413},
		{-0.6430479288101195},
		{4.2544732093811035},
		{1.4855430126190186},
		{-0.1708667129278183},
	}

	logger.Infof("sanity check distance: %v", referenceframe.InputsL2Distance(start[name], x))

	return fmt.Errorf("finish plan")
}

func frameSystemWithOnePart(ctx context.Context, myRobot robot.Robot, name string) (referenceframe.FrameSystem, error) {
	fsc, err := myRobot.FrameSystemConfig(ctx)
	if err != nil {
		return nil, err
	}

	parts := []*referenceframe.FrameSystemPart{}

	for name != "world" {
		p := findPart(fsc, name)
		if p == nil {
			return nil, fmt.Errorf("cannot find frame [%s]", name)
		}
		parts = append(parts, p)
		name = p.FrameConfig.Parent()
	}

	return referenceframe.NewFrameSystem("temp", parts, nil)
}

func findPart(fsc *framesystem.Config, name string) *referenceframe.FrameSystemPart {
	for _, c := range fsc.Parts {
		if c.FrameConfig.Name() == name {
			return c
		}
	}
	return nil
}
