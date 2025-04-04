package pour

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/golang/geo/r3"
	vizClient "github.com/viam-labs/motion-tools/client"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/spatialmath"
)

const (
	pourAngleSafe     = 0.5
	emptyBottleWeight = 675
	cupRadius         = 50 // mm
	heightAboveCup    = 50
)

//	var JointPositionsPickUp = referenceframe.FloatsToInputs([]float64{
//	 1.595939040184021,
//	 0.4438844323158264,
//	 -0.6554062962532043,
//	 1.5953776836395264,
//	 1.5655426979064941,
//	 -2.9301466941833496,
//	})
var JointPositionsHome = referenceframe.FloatsToInputs([]float64{
	1.5965231657028198,
	0.33616247773170466,
	-0.6152324676513673,
	1.5954617261886597,
	1.5636086463928223,
	-2.862619638442993,
})

//	var JointPositionsPreppingForPour = referenceframe.FloatsToInputs([]float64{
//	 4.104966640472412,
//	 -0.29940700531005854,
//	 -0.520090639591217,
//	 2.823283672332764,
//	 -0.40353041887283325,
//	 -2.629184246063233,
//	})
var JointPositionsPreppingForPour = referenceframe.FloatsToInputs([]float64{
	2.071307420730591, -0.39255717396736145, -0.5151031017303467, 1.8614627122879028, 1.1730695962905884, -2.2705657482147217,
})
var JointPositionsPreppingForPour2 = referenceframe.FloatsToInputs([]float64{
	3.9580442905426025, -0.189841628074646, -0.8737765550613403, 2.823736429214478, -0.42844274640083313, -2.629254579544068,
})
var JointPositionsScale = referenceframe.FloatsToInputs([]float64{
	2.4994733333587646, 0.6651768088340759, -1.2272746562957764, 2.416159152984619, 1.1277557611465454, -2.765428304672241,
})

func (g *Gen) demoPlanMovements(ctx context.Context, cupLocations []r3.Vector, options PouringOptions) error {
	if len(cupLocations) == 0 {
		return errors.New("no cups to pour for")
	}
	thePlan, err := g.startPlan(ctx)
	if err != nil {
		return err
	}
	// first we need to make sure that the griper is open
	thePlan.add(newGripperOpen(g.gripper))
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsHome))
	if options.PickupFromFar && options.PickupFromMid {
		return fmt.Errorf("cannot pickup from both locations")
	}
	// go get the bottle and move it to the scale
	var pickupPoint r3.Vector
	if options.PickupFromFar {
		pickupPoint = farBottlePickup
	}
	if options.PickupFromMid {
		pickupPoint = middleBottlePick
	}
	obstacles := GenerateObstacles()
	transforms := GenerateTransforms("world", g.arm.Name().ShortName(), spatialmath.NewPoseFromPoint(pickupPoint), pickupPoint, g.conf.BottleHeight)
	worldState, err := referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return fmt.Errorf("cannot create world state %v", err)
	}
	prepSpot := spatialmath.NewPose(r3.Vector{X: pickupPoint.X + 150, Y: pickupPoint.Y, Z: pickupPoint.Z}, grabVectorOrient)
	// move to prep spot
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), prepSpot, worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}
	prepJoints := thePlan.current()
	// move to actual spot
	transforms = GenerateTransforms("gripper", g.arm.Name().ShortName(), spatialmath.NewPoseFromPoint(pickupPoint), pickupPoint, g.conf.BottleHeight)
	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return fmt.Errorf("cannot create world state %v", err)
	}
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), spatialmath.NewPose(pickupPoint, grabVectorOrient), worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}
	wineJoints := thePlan.current()
	// grab bottle
	thePlan.add(newGripperGrab(g.gripper))
	// move to safety
	safety := spatialmath.NewPose(
		r3.Vector{X: pickupPoint.X, Y: pickupPoint.Y, Z: pickupPoint.Z + 200},
		grabVectorOrient,
	)
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), safety, worldState, &linearAndBottleConstraint, 0, 100)
	if err != nil {
		return err
	}
	safetyJoints := thePlan.current()
	// go to scale
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsScale))
	// drop on scale
	thePlan.add(newGripperOpen(g.gripper))
	// Define the resource names of bottle and gripper as they do not exist in the config
	bottleResource := resource.Name{Name: "bottle"}
	// gripperResource := resource.Name{Name: "gripper"}
	// transforms := GenerateTransforms("world", g.arm.Name().ShortName(), spatialmath.NewPoseFromPoint(wineBottleMeasurePoint), wineBottleMeasurePoint, g.conf.BottleHeight)
	// // GenerateObstacles returns a slice of geometries we are supposed to avoid at plan time
	// obstacles := GenerateObstacles()
	// // worldState combines the obstacles we wish to avoid at plan time with other frames (gripper & bottle) that are found on the robot
	// worldState, err := referenceframe.NewWorldState(obstacles, transforms)
	// if err != nil {
	//  return err
	// }
	// we need to do the plan thus far in order to weigh the bottle
	thePlan.do(ctx)
	// get the weight of the bottle
	var bottleWeight int
	for i := 0; i < 10; i++ {
		bottleWeight, err = g.getWeight(ctx)
		if err != nil {
			return err
		}
		time.Sleep(time.Millisecond * 10)
		if math.Abs(float64(bottleWeight)) < 400 {
			continue
		}
		break
	}
	g.logger.Infof("bottleWeight: %d", bottleWeight)
	if bottleWeight < emptyBottleWeight {
		thePlan.reverseDo(ctx)
		return errors.New("not enough liquid in bottle to pour into any of the given cups -- please refill the bottle")
	}
	// HACKY ALERT (sorry its late)
	// need to reset the plan now to not repeat steps
	thePlan, err = g.startPlan(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE FIRST PLAN
	// THE FIRST PLAN IS MOVING THE ARM TO BE IN THE NEUTRAL POSITION
	g.logger.Info("PLANNING the prep")
	// err = g.getPlanAndAdd(ctx, thePlan, gripperResource, spatialmath.NewPose(wineBottleMeasurePoint, grabVectorOrient), worldState, &linearAndBottleConstraint, 0, 100)
	// if err != nil {
	//  return err
	// }
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsScale))
	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE SECOND PLAN
	// THE SECOND PLAN MOVES THE GRIPPER TO A POSITION WHERE IT CAN GRASP THE BOTTLE
	// ENGAGE BOTTLE
	g.logger.Info("PLANNING FOR THE 2nd MOVEMENT")
	// err = g.getPlanAndAdd(ctx, thePlan, gripperResource, spatialmath.NewPose(wineBottleMeasurePoint, grabVectorOrient), worldState, &linearAndBottleConstraint, 0, 100)
	// if err != nil {
	//  return err
	// }
	// HERE WE CONSTRUCT THE THIRD PLAN
	// THE THIRD PLAN MOVES THE GRIPPER WHICH CLUTCHES THE BOTTLE INTO THE LIFTED GOAL POSITION
	// REDEFINE BOTTLE LINK TO BE ATTACHED TO GRIPPER
	thePlan.add(newGripperGrab(g.gripper))
	transforms = GenerateTransforms("gripper", g.arm.Name().ShortName(), spatialmath.NewPoseFromOrientation(grabVectorOrient), wineBottleMeasurePoint, g.conf.BottleHeight)
	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return err
	}
	// intermediate poses
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsPreppingForPour))
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsPreppingForPour2))
	// AT THIS POINT IN THE PLAN GENERATION, WE'VE LIFTED THE BOTTLE INTO THE ARM AND ARE NOW READY TO
	// MOVE IT TO THE POUR READY POSITION(S)
	// here we add the cups as obstacles to be avoided
	cupGeoms := []spatialmath.Geometry{}
	for i, cupLoc := range cupLocations {
		cupOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: cupLoc.X, Y: cupLoc.Y, Z: plywoodHeight})
		label := "cup" + strconv.Itoa(i)
		cupObj, _ := spatialmath.NewCapsule(cupOrigin, cupRadius, g.conf.CupHeight, label)
		cupGeoms = append(cupGeoms, cupObj)
	}
	cupGifs := referenceframe.NewGeometriesInFrame(referenceframe.World, cupGeoms)
	obstacles = append(obstacles, cupGifs)
	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		return err
	}
	fsCfg, _ := g.robotClient.FrameSystemConfig(ctx)
	parts := fsCfg.Parts
	fs, err := referenceframe.NewFrameSystem("newFS", parts, worldState.Transforms())
	if err != nil {
		return err
	}
	currentInputs, err := g.arm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}
	ee, err := g.arm.EndPosition(ctx, nil)
	if err != nil {
		return err
	}
	geometries, err := worldState.ObstaclesInWorldFrame(fs, map[string][]referenceframe.Input{g.conf.ArmName: currentInputs})
	if err != nil {
		return err
	}
	for _, g := range geometries.Geometries() {
		vizClient.DrawGeometry(g, "red")
	}
	for i := range transforms {
		smth := transforms[i]
		fmt.Println(smth.Name())
		if smth.Name() == "bottle" {
			vizClient.DrawGeometry(smth.Geometry().Transform(ee), "blue")
		}
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
			r3.Vector{cupLoc.X, cupLoc.Y, cupLoc.Z + heightAboveCup},
			&spatialmath.OrientationVectorDegrees{OX: pourVec.X, OY: pourVec.Y, OZ: pourAngleSafe, Theta: 179},
		)
		fmt.Println("planning pourReadyGoal")
		fmt.Println("pourReadyGoal: ", spatialmath.PoseToProtobuf(pourReadyGoal))
		err = g.getPlanAndAddForCup(ctx, thePlan, bottleResource, pourReadyGoal, worldState, &orientationConstraint)
		if err != nil {
			return fmt.Errorf("could not plan for cup %d even after retrying %v", i, err)
		}
		// testing section
		// thePlan.do(ctx)
		// panic("oops")
		pourGoal := spatialmath.NewPose(
			r3.Vector{X: cupLoc.X, Y: cupLoc.Y, Z: cupLoc.Z},
			&spatialmath.OrientationVectorDegrees{OX: pourVec.X, OY: pourVec.Y, OZ: pourParameters[0], Theta: 150},
		)
		fmt.Println("planning pourGoal")
		fmt.Println("pourGoal: ", spatialmath.PoseToProtobuf(pourGoal))
		p, err := g.getPlanByTryingRepeatedly(ctx, thePlan, bottleResource, pourGoal, worldState, &linearConstraint)
		if err != nil {
			return fmt.Errorf("cannot generate pour plan for cup %d %v", i, err)
		}
		thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), p))
		thePlan.add(newSleepAction(time.Millisecond * time.Duration(pourParameters[1]))) // pour
		thePlan.add(newSetSpeed(g.arm, 180, 180*2))                                      // we want to move fast to not spill
		thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), reversePlan(p)))
		thePlan.add(newSetSpeed(g.arm, 60, 100))
		g.setStatus(fmt.Sprintf("planned cup %d", i+1))
	}
	// back to the prep position
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsPreppingForPour2))
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsPreppingForPour))
	// move to above where the bottle is returned
	thePlan.add(newMoveToJointPositionsAction(g.arm, safetyJoints))
	// // move back to above rest position
	// plan, err := g.getPlanByTryingRepeatedly(ctx, thePlan, g.arm.Name(), spatialmath.NewPose(pickupPoint.Add(r3.Vector{0, 0, 200}), grabVectorOrient), worldState, &orientationConstraint)
	// if err != nil {
	//  return err
	// }
	// thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), plan))
	// return to the wine position
	thePlan.add(newMoveToJointPositionsAction(g.arm, wineJoints))
	// // place bottle on table
	// err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), spatialmath.NewPose(pickupPoint, grabVectorOrient), worldState, &tableAndBottleConstraint, 0, 100)
	// if err != nil {
	//  return err
	// }
	// open
	thePlan.add(newGripperOpen(g.gripper))
	// retreat to prep spot
	thePlan.add(newMoveToJointPositionsAction(g.arm, prepJoints))
	// retreat to neutral position
	err = g.getPlanAndAdd(ctx, thePlan, g.arm.Name(), spatialmath.NewPose(pickupPoint.Add(r3.Vector{X: 200}), grabVectorOrient), worldState, nil, 0, 100)
	if err != nil {
		return err
	}
	// and finally go back home
	thePlan.add(newMoveToJointPositionsAction(g.arm, JointPositionsHome))
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
	for i := 0; i < 100; i++ {
		p, err = g.getPlan(ctx, thePlan.current(), toMove, goal, worldState, constraint, i, 1000, 0.2)
		if err == nil {
			armInputs, _ := p.Trajectory().GetFrameInputs(g.arm.Name().ShortName())
			for i := range armInputs {
				penultimateJointPosition := armInputs[i][4].Value
				if penultimateJointPosition < 0 {
					thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), p))
					return nil
				}
			}
		}
	}
	return err
}
func (g *Gen) getPlanByTryingRepeatedly(ctx context.Context, thePlan *planBuilder, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints) (motionplan.Plan, error) {
	var err error
	var p motionplan.Plan
	for i := 0; i < 20; i++ {
		p, err = g.getPlan(ctx, thePlan.current(), toMove, goal, worldState, constraint, i, 1000, 2)
		if err == nil {
			return p, nil
		}
	}
	return nil, err
}
func (g *Gen) getPlanAndAdd(ctx context.Context, thePlan *planBuilder, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints, rseed, smoothIter int) error {
	p, err := g.getPlan(ctx, thePlan.current(), toMove, goal, worldState, constraint, rseed, smoothIter, 10)
	if err != nil {
		return err
	}
	thePlan.add(newMotionPlanAction(g.motion, g.arm.Name().ShortName(), p))
	return nil
}
func (g *Gen) getPlan(ctx context.Context, armCurrentInputs []referenceframe.Input, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints, rseed, smoothIter int, timeout float64) (motionplan.Plan, error) {
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
		Options:     map[string]interface{}{"rseed": rseed, "timeout": timeout, "smooth_iter": smoothIter, "num_threads": g.numThreads},
	})
}
