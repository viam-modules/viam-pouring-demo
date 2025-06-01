package main

import (
	"context"
	"fmt"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/robot"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func plan(ctx context.Context, myRobot robot.Robot, Cfg *pour.Config, p1c *pour.Pour1Components, logger logging.Logger) error {

	fs, err := frameSystemWithOnePart(ctx, myRobot, "arm-left")
	if err != nil {
		return err
	}

	fmt.Printf("hi %v\n", fs)

	req := &motionplan.PlanRequest{
		Logger:      logger,
		FrameSystem: fs,
	}

	fmt.Printf("req: %v\n", req)

	return fmt.Errorf("finish plan")
}

func frameSystemWithOnePart(ctx context.Context, myRobot robot.Robot, name string) (referenceframe.FrameSystem, error) {
	fsc, err := myRobot.FrameSystemConfig(ctx)
	if err != nil {
		return nil, err
	}

	for _, c := range fsc.Parts {
		if c.FrameConfig.Name() == name {
			return referenceframe.NewFrameSystem("temp", []*referenceframe.FrameSystemPart{c}, nil)
		}
	}

	return nil, fmt.Errorf("cannot find part with name [%s]", name)
}
