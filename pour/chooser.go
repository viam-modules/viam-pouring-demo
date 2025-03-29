package pour

import (
	"context"
	"fmt"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/services/motion/builtin"
	"go.viam.com/rdk/spatialmath"
)

var farBottlePickup = r3.Vector{X: -463, Y: 490, Z: 24}

func (g *Gen) PickFarBottle(ctx context.Context) error {

	return g.pickupBottle(ctx, farBottlePickup)
}

func (g *Gen) pickupBottle(ctx context.Context, pickupSpot r3.Vector) error {

	setup := r3.Vector{X: pickupSpot.X + 150, Y: pickupSpot.Y, Z: pickupSpot.Z}

	current, err := g.arm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	obstacles := []*referenceframe.GeometriesInFrame{}
	transforms := []*referenceframe.LinkInFrame{}

	worldState, err := referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return fmt.Errorf("cannot create world state %v", err)
	}

	plan, err := g.getPlan(ctx, current, g.arm.Name(), spatialmath.NewPoseFromPoint(setup), worldState, nil, 0, 100)
	if err != nil {
		return fmt.Errorf("failing to get plan: %v", err)
	}

	_, err = g.motion.DoCommand(ctx, map[string]interface{}{builtin.DoExecute: plan.Trajectory()})
	return err
}
