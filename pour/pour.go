package pour

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/spatialmath"
)

const (
	pourAngleSafe     = 0.5
	emptyBottleWeight = 675
)

var JointPositionsPickUp = referenceframe.FloatsToInputs([]float64{
	1.595939040184021,
	0.4438844323158264,
	-0.6554062962532043,
	1.5953776836395264,
	1.5655426979064941,
	-2.9301466941833496,
})

var JointPositionsPreppingForPour = referenceframe.FloatsToInputs([]float64{
	3.9929597377678049952,
	-0.31163778901022853862,
	-0.40986624359982865018,
	2.8722410201955117515,
	-0.28700971603322356085,
	-2.7665438651969944672,
})

func (g *Gen) demoPlanMovements(ctx context.Context, cupLocations []r3.Vector, options PouringOptions) error {

	thePlan, err := g.startPlan(ctx)
	if err != nil {
		return err
	}

	// first we need to make sure that the griper is open
	thePlan.add(newGripperOpen(g.gripper))

	prepPhaseStartPostion := thePlan.size()

	if options.PickupFromFar && options.PickupFromMid {
		return fmt.Errorf("cannot pickup from both locations")
	}

	if options.PickupFromFar {
		err = g.addBottleFetch(ctx, thePlan, farBottlePickup)
	}
	if options.PickupFromMid {
		err = g.addBottleFetch(ctx, thePlan, middleBottlePick)
	}

	// Define the resource names of bottle and gripper as they do not exist in the config
	bottleResource := resource.Name{Name: "bottle"}
	gripperResource := resource.Name{Name: "gripper"}

	transforms := GenerateTransforms("world", g.arm.Name().ShortName(), spatialmath.NewPoseFromPoint(wineBottleMeasurePoint), wineBottleMeasurePoint, g.conf.BottleHeight)

	// GenerateObstacles returns a slice of geometries we are supposed to avoid at plan time
	obstacles := GenerateObstacles()

	// worldState combines the obstacles we wish to avoid at plan time with other frames (gripper & bottle) that are found on the robot
	worldState, err := referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return err
	}

	// get the weight of the bottle
	bottleWeight, err := g.getWeight(ctx)
	if err != nil {
		return err
	}

	g.logger.Infof("bottleWeight: %d", bottleWeight)
	if bottleWeight < emptyBottleWeight {
		return errors.New("not enough liquid in bottle to pour into any of the given cups -- please refill the bottle")
	}

	now := time.Now()
	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE FIRST PLAN
	// THE FIRST PLAN IS MOVING THE ARM TO BE IN THE NEUTRAL POSITION
	g.logger.Info("PLANNING the prep")

	err = g.getPlanAndAdd(ctx, thePlan, gripperResource, spatialmath.NewPose(wineBottleMeasurePoint, grabVectorOrient), worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}

	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE SECOND PLAN
	// THE SECOND PLAN MOVES THE GRIPPER TO A POSITION WHERE IT CAN GRASP THE BOTTLE
	// ENGAGE BOTTLE
	g.logger.Info("PLANNING FOR THE 2nd MOVEMENT")

	err = g.getPlanAndAdd(ctx, thePlan, gripperResource, spatialmath.NewPose(wineBottleMeasurePoint, grabVectorOrient), worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}

	// HERE WE CONSTRUCT THE THIRD PLAN
	// THE THIRD PLAN MOVES THE GRIPPER WHICH CLUTCHES THE BOTTLE INTO THE LIFTED GOAL POSITION
	// REDEFINE BOTTLE LINK TO BE ATTACHED TO GRIPPER

	thePlan.add(newGripperGrab(g.gripper))

	transforms = GenerateTransforms("gripper", g.arm.Name().ShortName(), spatialmath.NewPoseFromOrientation(grabVectorOrient), wineBottleMeasurePoint, g.conf.BottleHeight)
	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return err
	}

	// LIFT
	g.logger.Info("PLANNING FOR THE 3rd MOVEMENT")
	liftedgoal := spatialmath.NewPose(
		r3.Vector{X: wineBottleMeasurePoint.X, Y: wineBottleMeasurePoint.Y, Z: wineBottleMeasurePoint.Z + 280},
		grabVectorOrient,
	)

	err = g.getPlanAndAdd(ctx, thePlan, gripperResource, liftedgoal, worldState, &bottleGripperSpec, 0, 100)
	if err != nil {
		return err
	}
	g.setStatus("done with prep planning")

	prepPhaseEndPosition := thePlan.size() - 1

	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsPreppingForPour))

	// AT THIS POINT IN THE PLAN GENERATION, WE'VE LIFTED THE BOTTLE INTO THE ARM AND ARE NOW READY TO
	// MOVE IT TO THE POUR READY POSITION(S)

	// here we add the cups as obstacles to be avoided
	cupGeoms := []spatialmath.Geometry{}
	for i, cupLoc := range cupLocations {
		cupOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: cupLoc.X, Y: cupLoc.Y, Z: 60})
		radius := 45.
		length := 170.
		label := "cup" + strconv.Itoa(i)
		cupObj, _ := spatialmath.NewCapsule(cupOrigin, radius, length, label)
		cupGeoms = append(cupGeoms, cupObj)
	}
	cupGifs := referenceframe.NewGeometriesInFrame(referenceframe.World, cupGeoms)
	obstacles = append(obstacles, cupGifs)
	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return err
	}

	for i, cupLoc := range cupLocations {
		currentBottleWeight := bottleWeight - (150 * i)
		g.logger.Infof("currentBottleWeight: %d", currentBottleWeight)

		// if there is not enough liquid in the bottle do not pour anything out
		if currentBottleWeight < emptyBottleWeight {
			g.logger.Info("there are still cups remaining but we will not pour into them since there is not enough liquid left in the bottle")
			break
		}

		pourParameters := getAngleAndSleep(currentBottleWeight)
		pourVec := cupLoc
		pourVec.Z = 0
		pourVec = pourVec.Normalize()

		// MOVE TO POUR READY POSE
		pourReadyGoal := spatialmath.NewPose(
			cupLoc,
			&spatialmath.OrientationVectorDegrees{OX: pourVec.X, OY: pourVec.Y, OZ: pourAngleSafe, Theta: 179},
		)

		err = g.getPlanAndAddForCup(ctx, thePlan, bottleResource, pourReadyGoal, worldState, &orientationConstraint)
		if err != nil {
			return fmt.Errorf("could not plan for cup %d even after retrying %v", i, err)
		}

		pourGoal := spatialmath.NewPose(
			r3.Vector{X: cupLoc.X, Y: cupLoc.Y, Z: cupLoc.Z - 20},
			&spatialmath.OrientationVectorDegrees{OX: pourVec.X, OY: pourVec.Y, OZ: pourParameters[0], Theta: 150},
		)
		p, err := g.getPlan(ctx, thePlan.current(), bottleResource, pourGoal, worldState, &linearConstraint, 0, 100)
		if err != nil {
			return fmt.Errorf("cannot generate pour plan for cup %d %v", i, err)
			return err
		}
		thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), p))

		thePlan.add(newSleepAction(time.Millisecond * time.Duration(pourParameters[1]))) // pour

		thePlan.add(newSetSpeed(g.arm, 180, 180*2)) // we want to move fast to not spill
		thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), reversePlan(p)))
		thePlan.add(newSetSpeed(g.arm, 60, 100))

		g.setStatus(fmt.Sprintf("planned cup %d", i+1))
	}

	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsPreppingForPour))

	// this should become a plan so that we not knock over cups
	// this is above the scale about 50cm
	thePlan.add(newMoveToJointPositionsAction(g.arm, referenceframe.FloatsToInputs([]float64{
		1.6003754138906833848,
		-0.39200037717721969432,
		-0.60418236255495871845,
		1.58686017989718664,
		1.5460307598075662128,
		-2.1456081867164793486,
	})))

	// go back home
	thePlan.addReverse(prepPhaseStartPostion, prepPhaseEndPosition)

	// open
	thePlan.add(newGripperOpen(g.gripper))

	g.logger.Infof("IT TOOK THIS LONG TO CONSTRUCT ALL PLANS: %v", time.Since(now))

	if !options.DoPour {
		g.logger.Infof("not moving")
		return nil
	}

	g.setStatus("DONE CONSTRUCTING PLANS -- EXECUTING NOW")

	err = thePlan.do(ctx)
	if err != nil {
		return err
	}

	g.setStatus("done running the demo")
	return nil
}

// Generate any transforms needed. Pass parent to parent the bottle to world or the arm
func GenerateTransforms(parent, armName string, pose spatialmath.Pose, bottlePosition r3.Vector, bottleHeight float64) []*referenceframe.LinkInFrame {

	transforms := []*referenceframe.LinkInFrame{}

	// frame 1
	bottleOffsetFrame := referenceframe.NewLinkInFrame(
		parent,
		pose,
		"bottle_offset",
		nil,
	)
	transforms = append(transforms, bottleOffsetFrame)

	// frame 2
	bottleCenterZ := bottleHeight / 2.

	bottleLinkLen := r3.Vector{X: 0, Y: 0, Z: bottleHeight - bottlePosition.Z}

	bottleGeom, _ := spatialmath.NewCapsule(spatialmath.NewPoseFromPoint(r3.Vector{X: 0, Y: 0, Z: -bottleCenterZ}), 35, 260, "bottle")

	bottleFrame := referenceframe.NewLinkInFrame(
		"bottle_offset",
		spatialmath.NewPoseFromPoint(bottleLinkLen),
		"bottle",
		bottleGeom,
	)
	transforms = append(transforms, bottleFrame)

	// frame 3
	gripperGeom, _ := spatialmath.NewBox(spatialmath.NewPoseFromPoint(r3.Vector{X: 0, Y: 0, Z: -80}), r3.Vector{X: 50, Y: 170, Z: 160}, "gripper")
	gripperFrame := referenceframe.NewLinkInFrame(
		armName,
		spatialmath.NewPoseFromPoint(r3.Vector{X: 0, Y: 0, Z: 150}),
		"gripper",
		gripperGeom,
	)
	transforms = append(transforms, gripperFrame)

	return transforms
}

func (g *Gen) getPlanAndAddForCup(ctx context.Context, thePlan *planBuilder, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints) error {
	var err error
	var p motionplan.Plan
	for i := 0; i < 20; i++ {
		p, err = g.getPlan(ctx, thePlan.current(), toMove, goal, worldState, constraint, i, 1000)
		if err == nil {
			armInputs, _ := p.Trajectory().GetFrameInputs(g.arm.Name().ShortName())
			penultimateJointPosition := armInputs[len(armInputs)-1][4].Value
			if penultimateJointPosition < 0 {
				thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), p))
				return nil
			}
		}
	}
	return err
}

func (g *Gen) getPlanAndAdd(ctx context.Context, thePlan *planBuilder, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints, rseed, smoothIter int) error {
	p, err := g.getPlan(ctx, thePlan.current(), toMove, goal, worldState, constraint, rseed, smoothIter)
	if err != nil {
		return err
	}
	thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), p))
	return nil
}

func (g *Gen) getPlan(ctx context.Context, armCurrentInputs []referenceframe.Input, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints, rseed, smoothIter int) (motionplan.Plan, error) {
	fsCfg, _ := g.robotClient.FrameSystemConfig(ctx)
	parts := fsCfg.Parts
	fs, err := referenceframe.NewFrameSystem("newFS", parts, worldState.Transforms())
	if err != nil {
		return nil, err
	}

	fsInputs := referenceframe.NewZeroInputs(fs)
	fsInputs[g.arm.Name().ShortName()] = armCurrentInputs
	g.logger.Infof("rseed: %d", rseed)

	return motionplan.PlanMotion(ctx, &motionplan.PlanRequest{
		Logger: g.logger,
		Goals: []*motionplan.PlanState{
			motionplan.NewPlanState(referenceframe.FrameSystemPoses{toMove.Name: referenceframe.NewPoseInFrame("world", goal)}, nil),
		},
		StartState:  motionplan.NewPlanState(nil, fsInputs),
		FrameSystem: fs,
		WorldState:  worldState,
		Constraints: constraint,
		Options:     map[string]interface{}{"rseed": rseed, "timeout": 10, "smooth_iter": smoothIter, "num_threads": g.numThreads},
	})
}
