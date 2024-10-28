package pour

import (
	"context"
	"fmt"
	"math"

	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/motion/builtin"
	"go.viam.com/rdk/spatialmath"
)

const (
	bottleHeight  = 310.
	pourAngleSafe = 0.5
)

var (
	armName = "arm"
)

func demoPlanMovements(machine *client.RobotClient, bottleGrabPoint r3.Vector, cupLocations []r3.Vector) {
	logger := logging.NewLogger("client")
	motionService, err := motion.FromRobot(machine, "builtin")
	if err != nil {
		logger.Fatal(err)
	}

	// Compute orientation to approach bottle. We may also just want to hardcode rather than depending on the start position
	vectorArmToBottle := r3.Vector{X: -1, Y: 0, Z: 0}
	grabVectorOrient := &spatialmath.OrientationVector{OX: vectorArmToBottle.X, OY: vectorArmToBottle.Y, OZ: vectorArmToBottle.Z}

	// DEFINE CONSTRAINTS HERE
	// Move linearly allowing no collisions
	linearConstraint := motionplan.Constraints{
		LinearConstraint: []motionplan.LinearConstraint{
			{LineToleranceMm: 5, OrientationToleranceDegs: 5},
		},
	}

	// Allow gripper-bottle collision to grab
	bottleGripperSpec := motionplan.Constraints{
		CollisionSpecification: []motionplan.CollisionSpecification{
			{Allows: []motionplan.CollisionSpecificationAllowedFrameCollisions{
				{Frame1: "gripper_origin", Frame2: "bottle_origin"},
			}},
		},
	}

	linearAndBottleConstraint := motionplan.Constraints{
		LinearConstraint: []motionplan.LinearConstraint{
			{LineToleranceMm: 1},
		},
		CollisionSpecification: []motionplan.CollisionSpecification{
			{Allows: []motionplan.CollisionSpecificationAllowedFrameCollisions{
				{Frame1: "gripper_origin", Frame2: "bottle_origin"},
			}},
		},
	}

	// Define an orientation constraint so that the bottle is not flipped over when moving
	orientationConst := motionplan.OrientationConstraint{OrientationToleranceDegs: 30}
	orientationConstraint := motionplan.NewConstraints(nil, []motionplan.OrientationConstraint{orientationConst}, nil)

	// Define the resource names of bottle and gripper as they do not exist in the config
	bottleResource := resource.Name{Name: "bottle"}
	gripperResource := resource.Name{Name: "gripper"}

	// GenerateTransforms adds the gripper and bottle frames
	transforms := GenerateTransforms("world", spatialmath.NewPoseFromPoint(bottleGrabPoint), bottleGrabPoint)

	// GenerateObstacles returns a slice of geometries we are supposed to avoid at plan time
	obstacles := GenerateObstacles()

	// worldState combines the obstacles we wish to avoid at plan time with other frames (gripper & bottle) that are found on the robot
	worldState, err := referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		logger.Fatal(err)
	}

	xArmComponent, err := arm.FromRobot(machine, armName)
	if err != nil {
		logger.Fatal(err)
		return
	}

	// get the weight of the bottle
	bottleWeight, err := getWeight(machine)
	if err != nil {
		logger.Fatal(err)
	}
	// bottleWeight += 500
	fmt.Println("bottleWeight: ", bottleWeight)

	bottleLocation := bottleGrabPoint
	approachgoal := spatialmath.NewPose(
		bottleLocation,
		grabVectorOrient,
	)

	bottleLocation = bottleGrabPoint
	bottleLocation.Z += 280
	liftedgoal := spatialmath.NewPose(
		bottleLocation,
		grabVectorOrient,
	)

	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE FIRST PLAN
	// THE FIRST PLAN IS MOVING THE ARM TO BE IN THE NEUTRAL POSITION
	fmt.Println("PLANNING FOR THE 1st MOVEMENT")
	armCurrentInputs, err := xArmComponent.CurrentInputs(context.Background())
	if err != nil {
		logger.Fatal(err)
	}

	approachGoalPlan, err := getPlan(context.Background(), logger, machine, armCurrentInputs, gripperResource, approachgoal, worldState, &linearAndBottleConstraint, 0)
	if err != nil {
		logger.Fatal(err)
	}
	fmt.Println("DONE PLANNING THE 1st MOVEMENT")
	fmt.Println(" ")
	// ---------------------------------------------------------------------------------

	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE SECOND PLAN
	// THE SECOND PLAN MOVES THE GRIPPER TO A POSITION WHERE IT CAN GRASP THE BOTTLE
	// ENGAGE BOTTLE
	fmt.Println("PLANNING FOR THE 2nd MOVEMENT")
	bottleLocation = bottleGrabPoint
	bottlegoal := spatialmath.NewPose(
		bottleLocation,
		grabVectorOrient,
	)

	// we need to adjust the fsInputs
	armFrameApproachGoalInputs, err := approachGoalPlan.Trajectory().GetFrameInputs(armName)
	if err != nil {
		logger.Fatal(err)
	}

	bottlePlan, err := getPlan(context.Background(), logger, machine, armFrameApproachGoalInputs[len(armFrameApproachGoalInputs)-1], gripperResource, bottlegoal, worldState, &linearAndBottleConstraint, 0)
	if err != nil {
		logger.Fatal(err)
	}
	fmt.Println("DONE PLANNING THE 2nd MOVEMENT")
	// ---------------------------------------------------------------------------------

	// ---------------------------------------------------------------------------------
	// HERE WE CONSTRUCT THE THIRD PLAN
	// THE THIRD PLAN MOVES THE GRIPPER WHICH CLUTCHES THE BOTTLE INTO THE LIFTED GOAL POSITION
	// REDEFINE BOTTLE LINK TO BE ATTACHED TO GRIPPER
	transforms = GenerateTransforms("gripper", spatialmath.NewPoseFromOrientation(grabVectorOrient), bottleGrabPoint)
	worldState, err = referenceframe.NewWorldState(obstacles, transforms)
	if err != nil {
		logger.Fatal(err)
	}

	// we need to adjust the fsInputs
	armFrameBottlePlanInputs, err := bottlePlan.Trajectory().GetFrameInputs(armName)
	if err != nil {
		logger.Fatal(err)
	}

	// LIFT
	fmt.Println("PLANNING FOR THE 3rd MOVEMENT")
	liftedPlan, err := getPlan(context.Background(), logger, machine, armFrameBottlePlanInputs[len(armFrameBottlePlanInputs)-1], gripperResource, liftedgoal, worldState, &bottleGripperSpec, 0)
	if err != nil {
		logger.Fatal(err)
	}
	fmt.Println("DONE PLANNING THE 3rd MOVEMENT")
	// ---------------------------------------------------------------------------------

	// AT THIS POINT IN THE PLAN GENERATION, WE'VE LIFTED THE BOTTLE INTO THE ARM AND ARE NOW READY TO
	// MOVE IT TO THE POUR READY POSITION(S)

	// ---------------------------------------------------------------------------------
	// NOTE: THIS WILL NEED TO BE UPDATED TO WORK FOR N CUPS AND NOT JUST ONE
	cupPouringPlans := make([]motionplan.Plan, len(cupLocations)*3)
	pourParams := make([][]float64, len(cupLocations))
	var getBackHomeCachedPlan motionplan.Plan

	for i, cupLoc := range cupLocations {
		minus := i * 150
		currentBottleWeight := bottleWeight - minus
		fmt.Println("currentBottleWeight: ", currentBottleWeight)
		pourParameters := getAngleAndSleep(currentBottleWeight)
		pourParams[i] = pourParameters
		pourVec := cupLoc
		pourVec.Z = 0
		pourVec = pourVec.Normalize()

		// MOVE TO POUR READY POSE
		pourReadyPt := cupLoc
		pourReadyGoal := spatialmath.NewPose(
			pourReadyPt,
			&spatialmath.OrientationVectorDegrees{OX: pourVec.X, OY: pourVec.Y, OZ: pourAngleSafe, Theta: 179},
		)

		// need to get the currentInputs for the arm
		var plan motionplan.Plan
		if i == 0 {
			armFrameLiftedPlanInputs, err := liftedPlan.Trajectory().GetFrameInputs(armName)
			if err != nil {
				logger.Fatal(err)
			}
			plan, err = getPlan(context.Background(), logger, machine, armFrameLiftedPlanInputs[len(armFrameLiftedPlanInputs)-1], bottleResource, pourReadyGoal, worldState, orientationConstraint, 0)
			if err != nil {
				logger.Fatal(err)
			}
			if getBackHomeCachedPlan == nil {
				j := 1
				for {
					fmt.Println("plan.Trajectory(): ", plan.Trajectory())
					armInputs, err := plan.Trajectory().GetFrameInputs(armName)
					if err != nil {
						logger.Fatal(err)
					}
					lastSetOfArmInputs := armInputs[len(armInputs)-1]
					// good plan
					// arm: [{1.9081637859344482} {-0.21761181950569153} {-0.6136443614959717} {1.802912712097168} {1.323805093765259} {-2.3392598628997807}]
					// arm: [{3.6838473236708578} {0.34870245978501047} {-1.2629047561914166} {3.0796384211177763} {-0.4517194870279333} {-3.066687865829873}]
					penUltimateJointPosition := lastSetOfArmInputs[len(lastSetOfArmInputs)-2]
					if penUltimateJointPosition.Value <= -0.1 {
						break
					}
					// the plan's traj had length not equal to 2 so we know it was not what we are looking for
					fmt.Println(" ")
					fmt.Println(" ")
					fmt.Println("WE ARE NOW GOING TO ASK FOR A NEW PATH SINCE WE DID NOT GET THE PATH WE WANTED!")
					fmt.Println(" ")
					fmt.Println(" ")
					plan, err = getPlan(context.Background(), logger, machine, armFrameLiftedPlanInputs[len(armFrameLiftedPlanInputs)-1], bottleResource, pourReadyGoal, worldState, orientationConstraint, j)
					if err != nil {
						logger.Fatal(err)
					}
					j++
				}
				getBackHomeCachedPlan = plan
			}
		} else {
			formerplan := cupPouringPlans[i*3-1]
			armFrameFormerPlanInputs, err := formerplan.Trajectory().GetFrameInputs(armName)
			if err != nil {
				logger.Fatal(err)
			}
			plan, err = getPlan(context.Background(), logger, machine, armFrameFormerPlanInputs[len(armFrameFormerPlanInputs)-1], bottleResource, pourReadyGoal, worldState, orientationConstraint, 0)
			if err != nil {
				logger.Fatal(err)
			}
		}
		cupPouringPlans[i*3] = plan

		// now we come up with the plan to actually pour the liquid
		// first we need to update the inputs tho
		armFramePlanInputs, err := plan.Trajectory().GetFrameInputs(armName)
		if err != nil {
			logger.Fatal(err)
		}

		pourPt := cupLoc
		pourGoal := spatialmath.NewPose(
			r3.Vector{X: pourPt.X, Y: pourPt.Y, Z: pourPt.Z - 100},
			&spatialmath.OrientationVectorDegrees{OX: pourVec.X, OY: pourVec.Y, OZ: pourParameters[0], Theta: 150},
		)
		plan, err = getPlan(context.Background(), logger, machine, armFramePlanInputs[len(armFramePlanInputs)-1], bottleResource, pourGoal, worldState, &linearConstraint, 0)
		if err != nil {
			logger.Fatal(err)
		}
		cupPouringPlans[i*3+1] = plan
		cupPouringPlans[i*3+2] = reversePlan(plan)
	}

	// ---------------------------------------------------------------------------------
	// AT THIS POINT IN TIME WE ARE DONE CONSTRUCTING ALL THE PLANS THAT WE WILL NEED AND NOW WE
	// WILL NEED TO RUN THEM ON THE ROBOT
	executeDemo(
		motionService,
		logger,
		xArmComponent,
		[]motionplan.Plan{approachGoalPlan, bottlePlan, liftedPlan},
		cupPouringPlans,
		[]motionplan.Plan{reversePlan(getBackHomeCachedPlan), reversePlan(liftedPlan), reversePlan(bottlePlan)},
		pourParams,
	)
}

func executeDemo(motionService motion.Service, logger logging.Logger, xArmComponent arm.Arm, beforePourPlans, pouringPlans, afterPourPlans []motionplan.Plan, pourParams [][]float64) {
	// NEED TO ADD LOGIC ON WHEN TO OPEN AND CLOSE THE GRIPPER
	// first we need to make sure that the griper is open
	// Open gripper
	xArmComponent.DoCommand(context.Background(), map[string]interface{}{
		"setup_gripper": true,
		"move_gripper":  850,
	})

	// plans which:
	// move the arm into the neutral position
	// move the arm to bottle
	// lift the bottle
	for i, plan := range beforePourPlans {
		cmd := map[string]interface{}{builtin.DoExecute: plan.Trajectory()}
		_, err := motionService.DoCommand(context.Background(), cmd)
		if err != nil {
			logger.Fatal(err)
		}
		if i == 1 {
			xArmComponent.DoCommand(context.Background(), map[string]interface{}{
				"setup_gripper": true,
				"move_gripper":  0,
			})
			time.Sleep(time.Second)
		}
	}

	// plans which:
	// move the bottle to be by the cup
	// move the bottle such that it pours liquid into the cups
	// move the bottle such that is is no longer pouring liquid
	for i, plan := range pouringPlans {
		if (i+1)%3 == 0 {
			fmt.Println("pourParams: ", pourParams)
			sleep := pourParams[i%2][1]
			fmt.Println("sleep: ", sleep)
			time.Sleep(time.Millisecond * time.Duration(sleep))
			// NOW WE SET THE SPEED AND ACCEL OF THE XARM TO 180 and 180*20
			_, err := xArmComponent.DoCommand(context.Background(), map[string]interface{}{
				"set_speed":        180,
				"set_acceleration": 180 * 20,
			})
			if err != nil {
				logger.Fatal(err)
			}
		}
		cmd := map[string]interface{}{builtin.DoExecute: plan.Trajectory()}
		_, err := motionService.DoCommand(context.Background(), cmd)
		if err != nil {
			logger.Fatal(err)
		}
		if (i+1)%3 == 0 {
			// NOW WE SET THE SPEED AND ACCEL OF THE ARM BACK TO 50 and 100
			_, err := xArmComponent.DoCommand(context.Background(), map[string]interface{}{
				"set_speed":        50,
				"set_acceleration": 100,
			})
			if err != nil {
				logger.Fatal(err)
			}
		}
	}

	for i, plan := range afterPourPlans {
		cmd := map[string]interface{}{builtin.DoExecute: plan.Trajectory()}
		_, err := motionService.DoCommand(context.Background(), cmd)
		if err != nil {
			logger.Fatal(err)
		}
		if i == 2 {
			xArmComponent.DoCommand(context.Background(), map[string]interface{}{
				"setup_gripper": true,
				"move_gripper":  850,
			})
		}
	}
}

// Generate any transforms needed. Pass parent to parent the bottle to world or the arm
func GenerateTransforms(parent string, pose spatialmath.Pose, bottleGrabPoint r3.Vector) []*referenceframe.LinkInFrame {
	bottleOffsetFrame := referenceframe.NewLinkInFrame(
		parent,
		pose,
		"bottle_offset",
		nil,
	)
	transforms := []*referenceframe.LinkInFrame{bottleOffsetFrame}

	bottleCenterZ := bottleHeight / 2.

	bottleLinkLen := r3.Vector{X: 0, Y: 0, Z: bottleHeight - bottleGrabPoint.Z}

	bottleGeom, _ := spatialmath.NewCapsule(spatialmath.NewPoseFromPoint(r3.Vector{0, 0, -bottleCenterZ}), 35, 260, "bottle")

	bottleFrame := referenceframe.NewLinkInFrame(
		"bottle_offset",
		spatialmath.NewPoseFromPoint(bottleLinkLen),
		"bottle",
		bottleGeom,
	)
	transforms = append(transforms, bottleFrame)

	gripperGeom, _ := spatialmath.NewBox(spatialmath.NewPoseFromPoint(r3.Vector{0, 0, -80}), r3.Vector{50, 170, 160}, "gripper")
	gripperFrame := referenceframe.NewLinkInFrame(
		armName,
		spatialmath.NewPoseFromPoint(r3.Vector{0, 0, 150}),
		"gripper",
		gripperGeom,
	)
	transforms = append(transforms, gripperFrame)

	return transforms
}

// Create the obstacles for things not to hit
func GenerateObstacles() []*referenceframe.GeometriesInFrame {
	obstaclesInFrame := []*referenceframe.GeometriesInFrame{}

	obstacles := []spatialmath.Geometry{}

	tableOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -428, Y: 0, Z: -510})
	tableDims := r3.Vector{X: 856, Y: 1170, Z: 960.0}
	tableObj, _ := spatialmath.NewBox(tableOrigin, tableDims, "table")
	obstacles = append(obstacles, tableObj)

	sideWallOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -428, Y: 585, Z: 0})
	sideWallDims := r3.Vector{X: 856, Y: 120, Z: 960.0}
	sideWallObj, _ := spatialmath.NewBox(sideWallOrigin, sideWallDims, "sideWall")
	obstacles = append(obstacles, sideWallObj)

	elevatedTableCenterOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -400, Y: 0, Z: 0})
	elevatedTableCenterDims := r3.Vector{X: 660, Y: 200, Z: 50.0}
	elevatedTableCenterObj, _ := spatialmath.NewBox(elevatedTableCenterOrigin, elevatedTableCenterDims, "elevatedTableCenter")
	obstacles = append(obstacles, elevatedTableCenterObj)

	protectPowerOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -125, Y: 0, Z: 0})
	protectPowerDims := r3.Vector{X: 150, Y: 130, Z: 100.0}
	protectPowerObj, _ := spatialmath.NewBox(protectPowerOrigin, protectPowerDims, "protectPowerImaginaryBox")
	obstacles = append(obstacles, protectPowerObj)

	wallOrigin := spatialmath.NewPoseFromPoint(r3.Vector{300, 0, 0})
	wallDims := r3.Vector{X: 10, Y: 2000, Z: 2000.0}
	wallObj, _ := spatialmath.NewBox(wallOrigin, wallDims, "wall")
	obstacles = append(obstacles, wallObj)

	ceilingOrigin := spatialmath.NewPoseFromPoint(r3.Vector{-400, 0, 900})
	ceilingDims := r3.Vector{X: 2000, Y: 2000, Z: 10.0}
	ceilingObj, _ := spatialmath.NewBox(ceilingOrigin, ceilingDims, "ceiling")
	obstacles = append(obstacles, ceilingObj)

	weightSensorOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: 515, Y: 325, Z: -10})
	weightSensorDims := r3.Vector{X: 177, Y: 152, Z: 58}
	weightSensorObj, _ := spatialmath.NewBox(weightSensorOrigin, weightSensorDims, "weightSensor")
	obstacles = append(obstacles, weightSensorObj)

	obstaclesInFrame = append(obstaclesInFrame, referenceframe.NewGeometriesInFrame(referenceframe.World, obstacles))

	return obstaclesInFrame
}

func getPlan(ctx context.Context, logger logging.Logger, machine *client.RobotClient, armCurrentInputs []referenceframe.Input, toMove resource.Name, goal spatialmath.Pose, worldState *referenceframe.WorldState, constraint *motionplan.Constraints, rseed int) (motionplan.Plan, error) {
	fsCfg, _ := machine.FrameSystemConfig(ctx)
	parts := fsCfg.Parts
	fs, _ := referenceframe.NewFrameSystem("newFS", parts, worldState.Transforms())

	fsInputs := referenceframe.StartPositions(fs)
	fsInputs[armName] = armCurrentInputs

	return motionplan.PlanMotion(ctx, &motionplan.PlanRequest{
		Logger:             logger,
		Goal:               referenceframe.NewPoseInFrame("world", goal),
		Frame:              fs.Frame(toMove.Name),
		StartConfiguration: fsInputs,
		FrameSystem:        fs,
		WorldState:         worldState,
		Constraints:        constraint,
		Options:            map[string]interface{}{"rseed": rseed},
	})
}

func reversePlan(originalPlan motionplan.Plan) motionplan.Plan {
	path := make(motionplan.Path, len(originalPlan.Path()))
	traj := make(motionplan.Trajectory, len(originalPlan.Trajectory()))

	// reverse the path
	for i, v := range originalPlan.Path() {
		path[len(originalPlan.Path())-1-i] = v
	}

	// reverse the traj
	for i, v := range originalPlan.Trajectory() {
		traj[len(originalPlan.Trajectory())-1-i] = v
	}
	return motionplan.NewSimplePlan(path, traj)
}

func getWeight(machine *client.RobotClient) (int, error) {
	wSensor1, _ := sensor.FromRobot(machine, "sensor-1")
	readings1, _ := wSensor1.Readings(context.Background(), nil)
	mass1 := readings1["mass_kg"].(float64)
	massInGrams1 := math.Round(mass1 * 1000)
	time.Sleep(time.Millisecond * 500)
	return int(massInGrams1), nil
}
