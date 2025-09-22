package pour

import (
	"context"
	"fmt"

	"github.com/golang/geo/r3"

	"github.com/erh/vmodutils"
	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/gripper"
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

func GetXArmGripperPosition(ctx context.Context, g gripper.Gripper) (int, error) {
	res, err := g.DoCommand(ctx, map[string]interface{}{
		"get": true,
	})
	if err != nil {
		return 0, err
	}
	pos, ok := vmodutils.GetIntFromMap(res, "pos")
	if !ok {
		return 0, fmt.Errorf("no pos in %v", res)
	}
	return pos, nil
}

// TODO HACK HACK HACK
// both caould be false meaning it's got something
// return pos, open, closed, error
func GetXArmGripperState(ctx context.Context, g gripper.Gripper) (int, bool, bool, error) {
	pos, err := GetXArmGripperPosition(ctx, g)
	if err != nil {
		return 0, false, false, err
	}
	if pos <= 10 {
		return pos, false, true, nil
	}
	if pos >= 830 {
		return pos, true, false, nil
	}
	return pos, false, false, nil
}

func CheckXArmGripperHasSomething(ctx context.Context, g gripper.Gripper) error {
	pos, open, closed, err := GetXArmGripperState(ctx, g)
	if err != nil {
		return err
	}
	if open || closed {
		return fmt.Errorf("gripper %v doesn't have something pos: %d open: %v closed: %v", g.Name(), pos, open, closed)
	}
	return nil
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

	inputs[j].Value += amount

	return a.MoveToJointPositions(ctx, inputs, nil)
}
