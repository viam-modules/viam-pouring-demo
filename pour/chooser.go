package pour

import (
	"context"
	"fmt"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/spatialmath"
)

var farBottlePickup = r3.Vector{X: -463, Y: 490, Z: 55}
var middleBottlePick = r3.Vector{X: -463, Y: 131, Z: 55}

func (g *Gen) PickFarBottle(ctx context.Context) error {
	return g.pickupBottle(ctx, farBottlePickup)
}

func (g *Gen) PickMiddleBottle(ctx context.Context) error {
	return g.pickupBottle(ctx, middleBottlePick)
}

func (g *Gen) pickupBottle(ctx context.Context, pickupSpot r3.Vector) error {

	thePlan, err := g.startPlan(ctx)
	if err != nil {
		return err
	}

	thePlan.add(newGripperOpen(g.gripper))

	startReverse := thePlan.size()

	err = g.addBottleFetch(ctx, thePlan, pickupSpot)
	if err != nil {
		return err
	}

	stopReverse := thePlan.size() - 1

	thePlan.addReverse(startReverse, stopReverse)

	return thePlan.do(ctx)
}

func (g *Gen) addBottleFetch(ctx context.Context, thePlan *planBuilder, pickupSpot r3.Vector) error {
	obstacles := GenerateObstacles()
	transforms := GenerateTransforms("world", g.arm.Name().ShortName(), spatialmath.NewPoseFromPoint(pickupSpot), pickupSpot, g.conf.BottleHeight)

	worldState, err := referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return fmt.Errorf("cannot create world state %v", err)
	}

	prepSpot := spatialmath.NewPose(r3.Vector{X: pickupSpot.X + 150, Y: pickupSpot.Y, Z: pickupSpot.Z}, grabVectorOrient)

	// move to prep spot
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), prepSpot, worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}

	// move to actual spot
	transforms = GenerateTransforms("gripper", g.arm.Name().ShortName(), spatialmath.NewPoseFromPoint(pickupSpot), pickupSpot, g.conf.BottleHeight)

	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return fmt.Errorf("cannot create world state %v", err)
	}
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), spatialmath.NewPose(pickupSpot, grabVectorOrient), worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}

	// grab bottle
	thePlan.add(newGripperGrab(g.gripper))

	// move to safety
	safety := spatialmath.NewPose(
		r3.Vector{X: pickupSpot.X, Y: pickupSpot.Y, Z: pickupSpot.Z + 200},
		grabVectorOrient,
	)
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), safety, worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}

	// go to scale
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsScale))

	// drop on scale
	thePlan.add(newGripperOpen(g.gripper))
	return nil
}
