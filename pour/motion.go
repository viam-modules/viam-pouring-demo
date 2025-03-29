package pour

import (
	"go.viam.com/rdk/motionplan"
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
