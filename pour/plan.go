package pour

import (
	"context"
	"fmt"
	"time"

	"go.viam.com/rdk/components/arm"
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
	if len(y) == 0 {
		panic("Wtf")
	}
	z := y[len(y)-1]
	if z == nil || len(z) != 6 {
		fmt.Printf("wtf %v %v\n", y, ma.plan)
		panic(2)
	}
	return z
}

func (ma *motionplanAction) do(ctx context.Context) error {
	_, err := ma.motion.DoCommand(ctx, map[string]interface{}{builtin.DoExecute: ma.plan.Trajectory()})
	return err
}

// ----
func newMoveToJointPositionsAction(a arm.Arm, joints []referenceframe.Input) action {
	return &moveToJointPositions{a, joints}
}

type moveToJointPositions struct {
	a      arm.Arm
	joints []referenceframe.Input
}

func (a *moveToJointPositions) isPlan() bool {
	return true
}
func (a *moveToJointPositions) position() []referenceframe.Input {
	return a.joints
}
func (a *moveToJointPositions) do(ctx context.Context) error {
	return a.a.MoveToJointPositions(ctx, a.joints, nil)
}

// ----
func newSetSpeed(a arm.Arm, speed, accel int) action {
	return &setSpeedAction{a, speed, accel}
}

type setSpeedAction struct {
	a            arm.Arm
	speed, accel int
}

func (a *setSpeedAction) isPlan() bool {
	return false
}
func (a *setSpeedAction) position() []referenceframe.Input {
	return nil
}
func (a *setSpeedAction) do(ctx context.Context) error {
	_, err := a.a.DoCommand(ctx, map[string]interface{}{
		"set_speed":        a.speed,
		"set_acceleration": a.accel,
	})
	return err
}

// ----

func newSleepAction(dur time.Duration) action {
	return &sleepAction{dur}
}

type sleepAction struct {
	dur time.Duration
}

func (sa *sleepAction) isPlan() bool {
	return false
}
func (sa *sleepAction) position() []referenceframe.Input {
	return nil
}
func (sa *sleepAction) do(ctx context.Context) error {
	time.Sleep(sa.dur)
	return nil
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
	got, err := gg.g.Grab(ctx, nil)
	if !got {
		return fmt.Errorf("didn't grab")
	}
	time.Sleep(time.Millisecond * 500) // TODO fix in ufactory gripper

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

// ----

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
			foo := x.position()
			if foo == nil || len(foo) != 6 {
				fmt.Printf("hi %#v\n", pb.plans[i])
				panic(1)
			}
			return foo
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
