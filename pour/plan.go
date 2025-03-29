package pour

import (
	"context"
	"time"

	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/motion/builtin"
)

type action interface {
	isPlan() bool
	position() []referenceframe.Input
	do(ctx context.Context) error
}

// ----

func newMotionPlanAction(motion motion.Service, frame string, plan motionplan.Plan) *motionplanAction {
	return &motionplanAction{motion: motion, frame: frame, plan: plan}
}

type motionplanAction struct {
	motion motion.Service
	frame  string
	plan   motionplan.Plan
}

func (ma *motionplanAction) isPlan() bool {
	return true
}

func (ma *motionplanAction) position() []referenceframe.Input {
	var y, err = ma.plan.Trajectory().GetFrameInputs(ma.frame)
	if err != nil {
		panic(err)
	}
	return y[len(y)-1]
}

func (ma *motionplanAction) do(ctx context.Context) error {
	_, err := ma.motion.DoCommand(ctx, map[string]interface{}{builtin.DoExecute: ma.plan.Trajectory()})
	return err
}

// ----

func newGripperGrab(g gripper.Gripper) action {
	return &gripperGrabAction{g}
}

type gripperGrabAction struct {
	g gripper.Gripper
}

func (gg *gripperGrabAction) isPlan() bool {
	return false
}
func (gg *gripperGrabAction) position() []referenceframe.Input {
	return nil
}
func (gg *gripperGrabAction) do(ctx context.Context) error {
	_, err := gg.g.Grab(ctx, nil)
	time.Sleep(time.Millisecond * 250) // TODO fix in ufactory gripper
	return err
}

func newGripperOpen(g gripper.Gripper) action {
	return &gripperOpenAction{g}
}

type gripperOpenAction struct {
	g gripper.Gripper
}

func (gg *gripperOpenAction) isPlan() bool {
	return false
}
func (gg *gripperOpenAction) position() []referenceframe.Input {
	return nil
}
func (gg *gripperOpenAction) do(ctx context.Context) error {
	return gg.g.Open(ctx, nil)
}

type planBuilder struct {
	start []referenceframe.Input
	plans []action
}

func newPlanBuilder(start []referenceframe.Input) *planBuilder {
	return &planBuilder{start: start}
}

func (pb *planBuilder) add(a action) {
	pb.plans = append(pb.plans, a)
}

func (pb *planBuilder) current() []referenceframe.Input {
	for i := len(pb.plans) - 1; i >= 0; i-- {
		var x = pb.plans[i]
		if x.isPlan() {
			return x.position()
		}
	}

	return pb.start
}

func (pb *planBuilder) do(ctx context.Context) error {
	for _, a := range pb.plans {
		err := a.do(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
