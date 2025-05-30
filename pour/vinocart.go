package pour

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/switch"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
	"go.viam.com/rdk/utils"
	viz "go.viam.com/rdk/vision"

	"github.com/erh/vmodutils"
)

var VinoCartModel = NamespaceFamily.WithModel("vinocart")

func init() {
	resource.RegisterService(generic.API, VinoCartModel, resource.Registration[resource.Resource, *Config]{Constructor: newVinoCart})
}

func newVinoCart(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	config, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
	}

	g := &VinoCart{
		name:   conf.ResourceName(),
		logger: logger,
		conf:   config,
	}

	g.c, err = Pour1ComponentsFromDependencies(config, deps)
	if err != nil {
		return nil, err
	}

	g.robotClient, err = vmodutils.ConnectToMachineFromEnv(ctx, logger)
	if err != nil {
		return nil, err
	}

	logger.Info("the pouring module has been constructed")
	return g, nil
}

func NewVinoCart(conf *Config, c *Pour1Components, client robot.Robot, logger logging.Logger) *VinoCart {
	return &VinoCart{
		conf:        conf,
		c:           c,
		robotClient: client,
		logger:      logger,
	}
}

type VinoCart struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	conf   *Config

	robotClient robot.Robot

	c *Pour1Components
}

func (vc *VinoCart) Name() resource.Name {
	return vc.name
}

func (vc *VinoCart) Close(ctx context.Context) error {
	return vc.robotClient.Close(ctx)
}

func (vc *VinoCart) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	if cmd["touch"] == true {
		return nil, vc.Touch(ctx)
	}

	if cmd["pour-prep"] == true {
		return nil, vc.PourPrep(ctx)
	}

	if cmd["pour"] == true {
		return nil, vc.Pour(ctx)
	}

	if cmd["put-back"] == true {
		return nil, vc.PutBack(ctx)
	}

	if cmd["demo"] == true {
		return nil, vc.FullDemo(ctx)
	}

	return nil, fmt.Errorf("need a command")
}

func (vc *VinoCart) FullDemo(ctx context.Context) error {
	err := vc.Touch(ctx)
	if err != nil {
		return err
	}
	err = vc.PourPrep(ctx)
	if err != nil {
		return err
	}
	err = vc.Pour(ctx)
	if err != nil {
		return err
	}
	return vc.PutBack(ctx)
}

func (vc *VinoCart) Touch(ctx context.Context) error {
	vc.logger.Infof("touch called")

	err := vc.c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	objects, err := vc.c.CupFinder.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return err
	}

	vc.logger.Infof("num objects: %v", len(objects))
	for _, o := range objects {
		vc.logger.Infof("\t objects: %v", o)
	}

	if len(objects) == 0 {
		return fmt.Errorf("no objects")
	}

	if len(objects) > 1 {
		return fmt.Errorf("too many objects %d", len(objects))
	}

	if vc.conf.SimoneHack {
		err = vc.c.LeftRetreat.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}

		err = vc.c.LeftPlace.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}

		_, err = vc.c.Gripper.Grab(ctx, nil)
		if err != nil {
			return err
		}

		return nil
	}

	obj := objects[0]

	// -- approach

	goToPose := vc.getApproachPoint(obj, 100, 0)
	vc.logger.Infof("going to move to %v", goToPose)

	obstacles := []*referenceframe.GeometriesInFrame{}
	obstacles = append(obstacles, referenceframe.NewGeometriesInFrame("world", []spatialmath.Geometry{obj.Geometry}))
	vc.logger.Infof("add cup as obstacle %v", obj.Geometry)

	worldState, err := referenceframe.NewWorldState(obstacles, nil)
	if err != nil {
		return err
	}

	_, err = vc.c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: vc.c.Gripper.Name().ShortName()},
			Destination:   goToPose,
			WorldState:    worldState,
		},
	)
	if err != nil {
		return err
	}

	// ---- go to pick up

	goToPose = vc.getApproachPoint(obj, -30, 0)
	vc.logger.Infof("going to move to %v", goToPose)

	_, err = vc.c.Motion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: vc.c.Gripper.Name().ShortName()},
			Destination:   goToPose,
			Constraints:   &LinearConstraint,
		},
	)
	if err != nil {
		return err
	}

	_, err = vc.c.Gripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) getApproachPoint(obj *viz.Object, deltaX, deltaZ float64) *referenceframe.PoseInFrame {
	md := obj.MetaData()
	c := md.Center()

	approachPoint := r3.Vector{
		Y: c.Y,
		Z: 95 + deltaZ,
	}

	if md.MinX > 0 {
		approachPoint.X = md.MinX - deltaX
	} else {
		approachPoint.X = md.MaxX + deltaX
	}

	return referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(
			approachPoint,
			&spatialmath.OrientationVectorDegrees{OX: 1, Theta: 180}),
	)

}

func (vc *VinoCart) doAll(ctx context.Context, all []toggleswitch.Switch) error {
	for _, s := range all {
		err := s.SetPosition(ctx, 2, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vc *VinoCart) pourPrepGrab(ctx context.Context) error {

	positions, err := vc.c.BottleArm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	orig := positions[0]

	vc.logger.Infof("pourPrepGrab orig: %v", orig)
	positions[0].Value -= utils.DegToRad(2)
	vc.logger.Infof("pourPrepGrab hack: %v", positions[0])

	err = vc.c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)

	_, err = vc.c.BottleGripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond)

	positions[0] = orig
	err = vc.c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) PourPrep(ctx context.Context) error {
	err := vc.doAll(ctx, vc.c.RightBottlePourPreGrabActions)
	if err != nil {
		return err
	}

	err = vc.pourPrepGrab(ctx)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, vc.c.RightBottlePourPostGrabActions)
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) Pour(ctx context.Context) error {
	positions, err := vc.c.BottleArm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	positionsLeft, err := vc.c.Arm.JointPositions(ctx, nil)
	if err != nil {
		return err
	}

	err = SetXarmSpeed(ctx, vc.c.BottleArm, 20, 100) // slow down
	if err != nil {
		return err
	}

	orig := positions[5]

	positions[5].Value = utils.DegToRad(-170)

	err = vc.c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)

	positions[5] = orig

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = vc.c.BottleArm.MoveToJointPositions(ctx, positions, nil)
		vc.logger.Errorf("error tilting bottle: %v", err)
	}()

	{
		err = SetXarmSpeed(ctx, vc.c.Arm, 20, 100) // back to default
		if err != nil {
			return err
		}

		positionsLeft[5].Value -= utils.DegToRad(-15)
		err = vc.c.Arm.MoveToJointPositions(ctx, positionsLeft, nil)
		if err != nil {
			return err
		}

		err = SetXarmSpeed(ctx, vc.c.Arm, 60, 100) // back to default
		if err != nil {
			return err
		}

	}

	wg.Wait()

	err = SetXarmSpeed(ctx, vc.c.BottleArm, 60, 100) // back to default
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) PutBack(ctx context.Context) error {
	x := append([]toggleswitch.Switch{}, vc.c.RightBottlePourPreGrabActions...)
	slices.Reverse(x)

	err := x[0].SetPosition(ctx, 2, nil)
	if err != nil {
		return err
	}

	err = vc.c.BottleGripper.Open(ctx, nil)
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 500)

	err = vc.doAll(ctx, x)

	err = vc.c.LeftPlace.SetPosition(ctx, 2, nil)
	if err != nil {
		return err
	}

	err = vc.c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	err = vc.c.LeftRetreat.SetPosition(ctx, 2, nil)
	if err != nil {
		return err
	}

	// just to get arms back to home
	_, err = vc.c.CupFinder.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return err
	}

	return nil
}
