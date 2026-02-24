package pour

import (
	"context"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
)

// Move linearly allowing no collisions
var LinearConstraint = motionplan.Constraints{
	LinearConstraint: []motionplan.LinearConstraint{
		{LineToleranceMm: 5, OrientationToleranceDegs: 5},
	},
}

func SetXarmSpeed(ctx context.Context, a arm.Arm, speed, accel float64) error {
	_, err := a.DoCommand(ctx, map[string]interface{}{
		"set_speed":        float64(speed),
		"set_acceleration": float64(accel),
	})
	return err
}

func SetXarmSpeedLog(ctx context.Context, a arm.Arm, speed, accel float64, logger logging.Logger) {
	err := SetXarmSpeed(ctx, a, speed, accel)
	if err != nil {
		logger.Errorf("SetXarmSpeed failed: %v", err)
	}
}

func Jog(ctx context.Context, m motion.Service, n resource.Name, j r3.Vector) error {
	pif, err := m.GetPose(ctx, n.ShortName(), "world", nil, nil)
	if err != nil {
		return err
	}

	goTo := referenceframe.NewPoseInFrame("world",
		spatialmath.NewPose(
			pif.Pose().Point().Add(j),
			pif.Pose().Orientation(),
		),
	)

	return moveWithLinearConstraint(ctx, m, n, goTo)
}

func JogJoint(ctx context.Context, a arm.Arm, j int, amount float64) error {
	inputs, err := a.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	inputs[j] += amount

	return a.MoveToJointPositions(ctx, inputs, nil)
}
