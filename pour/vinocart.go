package pour

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"go.uber.org/multierr"

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

	err := vc.doAll(ctx, "touch", "prep")
	if err != nil {
		return err
	}

	err = vc.c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	err = vc.c.BottleGripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	if vc.conf.SimoneHack {
		err = vc.doAll(ctx, "touch", "pickup-hack")
		if err != nil {
			return err
		}

		_, err = vc.c.Gripper.Grab(ctx, nil)
		if err != nil {
			return err
		}

		return nil
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

	obj := objects[0]

	// -- setup world frame

	obstacles := []*referenceframe.GeometriesInFrame{}
	obstacles = append(obstacles, referenceframe.NewGeometriesInFrame("world", []spatialmath.Geometry{obj.Geometry}))
	vc.logger.Infof("add cup as obstacle %v", obj.Geometry)

	worldState, err := referenceframe.NewWorldState(obstacles, nil)
	if err != nil {
		return err
	}

	// -- approach

	var o *spatialmath.OrientationVectorDegrees

	choices := []*spatialmath.OrientationVectorDegrees{
		{OX: 1, Theta: 180},
		{OX: 1, OY: 1, Theta: 180},
		{OY: 1, Theta: 180},
		{OX: -1, Theta: 180},
		{OX: -1, OY: -1, Theta: 180},
		{OY: -1, Theta: 180},
	}

	for _, tryO := range choices {
		goToPose := vc.getApproachPoint(obj, 100, tryO)
		vc.logger.Infof("trying to move to %v", goToPose)

		_, err = vc.c.Motion.Move(
			ctx,
			motion.MoveReq{
				ComponentName: resource.Name{Name: vc.c.Gripper.Name().ShortName()},
				Destination:   goToPose,
				WorldState:    worldState,
			},
		)
		if err == nil {
			o = tryO
			break
		}
	}

	if err != nil {
		return err
	}

	// ---- go to pick up

	goToPose := vc.getApproachPoint(obj, -80, o)
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

	// actual grab

	_, err = vc.c.Gripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) getApproachPoint(obj *viz.Object, deltaLinear float64, o *spatialmath.OrientationVectorDegrees) *referenceframe.PoseInFrame {
	md := obj.MetaData()

	d := math.Pow((o.OX*o.OX)+(o.OY*o.OY), .5)

	approachPoint := r3.Vector{
		Z: vc.conf.CupHeight - 25,
	}

	xLinear := (o.OX * deltaLinear / d)
	yLinear := (o.OY * deltaLinear / d)

	vc.logger.Infof("xLinear: %0.2f yLinear: %0.2f", xLinear, yLinear)

	if md.MinX > 0 {
		approachPoint.X = md.MinX - xLinear
	} else {
		approachPoint.X = md.MaxX + xLinear
	}

	if md.MinY > 0 {
		approachPoint.Y = md.MinY - yLinear
	} else {
		approachPoint.Y = md.MaxY + yLinear
	}

	return referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(approachPoint, o),
	)

}

func (vc *VinoCart) doAll(ctx context.Context, stage, step string) error {
	steps, ok := vc.c.Positions[stage]
	if !ok {
		return fmt.Errorf("no stage %s", stage)
	}
	positions, ok := steps[step]
	if !ok {
		return fmt.Errorf("no step [%s] in stage [%s]", step, stage)
	}

	for _, xxx := range positions {
		err := vc.goTo(ctx, xxx...)
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
	err := vc.doAll(ctx, "pour_prep", "prep-grab")
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "pour_prep", "right-grab")
	if err != nil {
		return err
	}

	err = vc.pourPrepGrab(ctx)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "pour_prep", "post-grab")
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) goTo(ctx context.Context, poss ...toggleswitch.Switch) error {
	if len(poss) == 0 {
		return nil
	}

	if len(poss) == 1 {
		return poss[0].SetPosition(ctx, 2, nil)
	}

	var errorLock sync.Mutex
	errors := []error{}

	wg := sync.WaitGroup{}

	for _, p := range poss {
		wg.Add(1)
		go func(pp toggleswitch.Switch) {
			defer wg.Done()
			err := pp.SetPosition(ctx, 2, nil)
			if err != nil {
				errorLock.Lock()
				errors = append(errors, err)
				errorLock.Unlock()
			}
		}(p)

	}

	wg.Wait()

	return multierr.Combine(errors...)
}

func (vc *VinoCart) DebugGetGlassPourCamImage(ctx context.Context, loopNumber int) (image.Image, string, error) {
	nimgs, _, err := vc.c.GlassPourCam.Images(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(nimgs) == 0 {
		return nil, "", fmt.Errorf("GlassPourCam returned no images")
	}

	fn := ""
	if loopNumber >= 0 {
		fn, err = saveImage(nimgs[0].Image, loopNumber)
		if err != nil {
			return nil, "", err
		}
	}

	return nimgs[0].Image, fn, nil
}

func (vc *VinoCart) Pour(ctx context.Context) error {
	err := vc.doAll(ctx, "pour", "prep")
	if err != nil {
		return err
	}

	defer func() {
		err := vc.doAll(ctx, "pour", "finish")
		if err != nil {
			vc.logger.Errorf("error trying to clean up Pour: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	bottleName := "bottle-top"
	bottleTop := referenceframe.NewLinkInFrame(
		vc.conf.BottleGripper,
		spatialmath.NewPose(r3.Vector{vc.conf.BottleHeight - 70, -7, 0}, &spatialmath.OrientationVectorDegrees{OX: 1}),
		bottleName,
		nil,
	)

	extraFrames := []*referenceframe.LinkInFrame{bottleTop}

	bottleStart, err := vc.c.BottleMotionService.GetPose(ctx, resource.Name{Name: bottleName}, "world", extraFrames, nil)
	if err != nil {
		return err
	}

	vc.logger.Infof("bottleStart: %v", bottleStart.Pose())

	worldState, err := referenceframe.NewWorldState(nil, extraFrames)
	if err != nil {
		return err
	}

	o := bottleStart.Pose().Orientation().OrientationVectorDegrees()

	start := time.Now()
	lastMove := time.Now().Add(-1 * time.Hour)

	poses := [][]referenceframe.Input{}

	loopNumber := 0

	var pd *pourDetector

	totalTime := 5 * time.Minute
	markedDifferent := false

	for time.Since(start) < totalTime && o.OZ > -0.5 {
		loopStart := time.Now()

		if time.Since(lastMove) > (time.Millisecond * 300) {
			o.OZ -= .05

			if o.OZ <= -0.15 && !markedDifferent {
				// when we get near empty, we need to go faster
				o.OZ -= .05
			}

			goalPose := referenceframe.NewPoseInFrame("world",
				spatialmath.NewPose(
					bottleStart.Pose().Point(),
					o,
				),
			)

			vc.logger.Infof("adjusting OZ to: %0.2f", o.OZ)

			_, err = vc.c.BottleMotionService.Move(
				ctx,
				motion.MoveReq{
					ComponentName: resource.Name{Name: bottleName},
					Destination:   goalPose,
					WorldState:    worldState,
				},
			)
			if err != nil {
				return err
			}

			lastMove = time.Now()

			inputs, err := vc.c.BottleArm.JointPositions(ctx, nil)
			if err != nil {
				return err
			}
			poses = append(poses, inputs)
		}

		img, fn, err := vc.DebugGetGlassPourCamImage(ctx /* loopNumber */, -1)
		if err != nil {
			return err
		}

		if pd == nil {
			pd = newPourDetector(img)
		} else {
			delta, _ := pd.differentDebug(img)
			deltaMax := 2.5
			vc.logger.Infof("fn: %v delta: %0.2f (%f)", fn, delta, deltaMax)
			if delta >= deltaMax && !markedDifferent {
				markedDifferent = true
				totalTime = time.Since(start) + time.Second
			}
		}

		sleepTime := (200 * time.Millisecond) - time.Since(loopStart)
		vc.logger.Debugf("going to sleep for %v", sleepTime)
		time.Sleep(sleepTime)
		loopNumber++
	}

	slices.Reverse(poses)

	err = vc.c.BottleArm.MoveThroughJointPositions(ctx, poses, nil, nil)
	if err != nil {
		return err
	}

	return vc.doAll(ctx, "pour", "finish")
}

func saveImage(img image.Image, loopNumber int) (string, error) {
	fn := fmt.Sprintf("img-%d.png", loopNumber)

	file, err := os.Create(fn)
	if err != nil {
		return fn, fmt.Errorf("couldn't create filename %w", err)
	}
	defer file.Close()
	return fn, png.Encode(file, img)
}

func (vc *VinoCart) PutBack(ctx context.Context) error {
	err := vc.doAll(ctx, "put-back", "before-open")
	if err != nil {
		return err
	}

	err = vc.c.BottleGripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	err = vc.c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	time.Sleep(time.Millisecond * 500)

	return vc.doAll(ctx, "put-back", "post-open")
}
