package pour

import (
	"github.com/golang/geo/r3"
)

var farBottlePickup = r3.Vector{X: -463, Y: 490, Z: 80}
var middleBottlePick = r3.Vector{X: -463, Y: 131, Z: 80}

// func (g *Gen) PickFarBottle(ctx context.Context) error {
// 	return g.pickupBottle(ctx, farBottlePickup)
// }

// func (g *Gen) PickMiddleBottle(ctx context.Context) error {
// 	return g.pickupBottle(ctx, middleBottlePick)
// }

// func (g *Gen) pickupBottle(ctx context.Context, pickupSpot r3.Vector) error {

// 	thePlan, err := g.startPlan(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	thePlan.add(newGripperOpen(g.gripper))

// 	startReverse := thePlan.size()

// 	err = g.addBottleFetch(ctx, thePlan, pickupSpot)
// 	if err != nil {
// 		return err
// 	}

// 	stopReverse := thePlan.size() - 1

// 	thePlan.addReverse(startReverse, stopReverse)

// 	return thePlan.do(ctx)
// }
