package pour

import (
	"github.com/golang/geo/r3"

	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/spatialmath"
)

// Move linearly allowing no collisions
var linearConstraint = motionplan.Constraints{
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

// Compute orientation to approach bottle. We may also just want to hardcode rather than depending on the start position
var vectorArmToBottle = r3.Vector{X: -1, Y: 0, Z: 0}
var grabVectorOrient = &spatialmath.OrientationVector{OX: vectorArmToBottle.X, OY: vectorArmToBottle.Y, OZ: vectorArmToBottle.Z}

// HARDCODE FOR NOW
// where to measure the wine bottled
var wineBottleMeasurePoint = r3.Vector{X: -255, Y: 334, Z: 108}
