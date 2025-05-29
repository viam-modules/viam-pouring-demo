package pour

import (
	"context"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/spatialmath"
)

const plywoodHeight = 32.8 // mm above the recessed part of table

// Move linearly allowing no collisions
var LinearConstraint = motionplan.Constraints{
	LinearConstraint: []motionplan.LinearConstraint{
		{LineToleranceMm: 5, OrientationToleranceDegs: 5},
	},
}

// Allow gripper-bottle collision to grab
var bottleGripperSpec = motionplan.Constraints{
	CollisionSpecification: []motionplan.CollisionSpecification{
		{Allows: []motionplan.CollisionSpecificationAllowedFrameCollisions{
			{Frame1: "gripper_origin", Frame2: "bottle_origin"},
		}},
	},
}

var linearAndBottleConstraint = motionplan.Constraints{
	LinearConstraint: []motionplan.LinearConstraint{
		{LineToleranceMm: 1},
	},
	CollisionSpecification: []motionplan.CollisionSpecification{
		{Allows: []motionplan.CollisionSpecificationAllowedFrameCollisions{
			{Frame1: "gripper_origin", Frame2: "bottle_origin"},
		}},
	},
}

var tableAndBottleConstraint = motionplan.Constraints{
	LinearConstraint: []motionplan.LinearConstraint{
		{LineToleranceMm: 1},
	},
	CollisionSpecification: []motionplan.CollisionSpecification{
		{Allows: []motionplan.CollisionSpecificationAllowedFrameCollisions{
			{Frame1: "table", Frame2: "bottle_origin"},
		}},
	},
	OrientationConstraint: []motionplan.OrientationConstraint{
		{OrientationToleranceDegs: 30},
	},
}

// Define an orientation constraint so that the bottle is not flipped over when moving
var orientationConstraint = motionplan.Constraints{
	OrientationConstraint: []motionplan.OrientationConstraint{
		{OrientationToleranceDegs: 30},
	},
}

// Compute orientation to approach bottle. We may also just want to hardcode rather than depending on the start position
var vectorArmToBottle = r3.Vector{X: -1, Y: 0, Z: 0}
var grabVectorOrient = &spatialmath.OrientationVector{OX: vectorArmToBottle.X, OY: vectorArmToBottle.Y, OZ: vectorArmToBottle.Z}

// HARDCODE FOR NOW
// where to measure the wine bottled
// var wineBottleMeasurePoint = r3.Vector{X: -255, Y: 334, Z: 108 - 29}
var wineBottleMeasurePoint = r3.Vector{X: -470, Y: 305, Z: 138}

// Create the obstacles for things not to hit
func GenerateObstacles() []*referenceframe.GeometriesInFrame {
	obstacles := []spatialmath.Geometry{}

	// these obstacles are for if there is no plywood on the table
	// tableOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -428, Y: 0, Z: -550})
	// tableDims := r3.Vector{X: 856, Y: 1170, Z: 960.0}
	// tableObj, _ := spatialmath.NewBox(tableOrigin, tableDims, "table")
	// obstacles = append(obstacles, tableObj)
	// elevatedTableCenterOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -400, Y: 0, Z: 0})
	// elevatedTableCenterDims := r3.Vector{X: 660, Y: 100, Z: 25.0}
	// elevatedTableCenterObj, _ := spatialmath.NewBox(elevatedTableCenterOrigin, elevatedTableCenterDims, "elevatedTableCenter")
	// obstacles = append(obstacles, elevatedTableCenterObj)

	// obstacle for if the table is covered by plywood
	tableOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -428, Y: 0, Z: -550 + plywoodHeight})
	tableDims := r3.Vector{X: 856, Y: 1170, Z: 960.0}
	tableObj, _ := spatialmath.NewBox(tableOrigin, tableDims, "table")
	obstacles = append(obstacles, tableObj)

	sideWallOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -428, Y: 655, Z: 0})
	sideWallDims := r3.Vector{X: 856, Y: 120, Z: 960.0}
	sideWallObj, _ := spatialmath.NewBox(sideWallOrigin, sideWallDims, "sideWall")
	obstacles = append(obstacles, sideWallObj)

	protectPowerOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -125, Y: 0, Z: 0})
	protectPowerDims := r3.Vector{X: 150, Y: 130, Z: 100.0}
	protectPowerObj, _ := spatialmath.NewBox(protectPowerOrigin, protectPowerDims, "protectPowerImaginaryBox")
	obstacles = append(obstacles, protectPowerObj)

	wallOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: 300, Y: 0, Z: 0})
	wallDims := r3.Vector{X: 10, Y: 2000, Z: 2000.0}
	wallObj, _ := spatialmath.NewBox(wallOrigin, wallDims, "wall")
	obstacles = append(obstacles, wallObj)

	ceilingOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: -400, Y: 0, Z: 900})
	ceilingDims := r3.Vector{X: 2000, Y: 2000, Z: 10.0}
	ceilingObj, _ := spatialmath.NewBox(ceilingOrigin, ceilingDims, "ceiling")
	obstacles = append(obstacles, ceilingObj)

	weightSensorOrigin := spatialmath.NewPoseFromPoint(r3.Vector{X: 515, Y: 325, Z: -10})
	weightSensorDims := r3.Vector{X: 177, Y: 152, Z: 58}
	weightSensorObj, _ := spatialmath.NewBox(weightSensorOrigin, weightSensorDims, "weightSensor")
	obstacles = append(obstacles, weightSensorObj)

	obstaclesInFrame := []*referenceframe.GeometriesInFrame{}
	obstaclesInFrame = append(obstaclesInFrame, referenceframe.NewGeometriesInFrame(referenceframe.World, obstacles))

	return obstaclesInFrame
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

func (g *Gen) startPlan(ctx context.Context) (*planBuilder, error) {
	current, err := g.c.Arm.JointPositions(ctx, nil)
	if err != nil {
		return nil, err
	}
	return newPlanBuilder(current), nil
}
