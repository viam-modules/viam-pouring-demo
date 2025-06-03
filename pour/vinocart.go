package pour

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
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
	"github.com/erh/vmodutils/touch"
)

const bottleName = "bottle-top"

var VinoCartModel = NamespaceFamily.WithModel("vinocart")

func init() {
	resource.RegisterService(generic.API, VinoCartModel, resource.Registration[resource.Resource, *Config]{Constructor: newVinoCart})
}

func newVinoCart(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	config, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
	}

	c, err := Pour1ComponentsFromDependencies(config, deps)
	if err != nil {
		return nil, err
	}

	robotClient, err := vmodutils.ConnectToMachineFromEnv(ctx, logger)
	if err != nil {
		return nil, err
	}

	g, err := NewVinoCart(ctx, config, c, robotClient, logger)
	if err != nil {
		return nil, err
	}

	logger.Info("the pouring module has been constructed")
	return g, nil
}

func NewVinoCart(ctx context.Context, conf *Config, c *Pour1Components, client robot.Robot, logger logging.Logger) (*VinoCart, error) {
	vc := &VinoCart{
		conf:        conf,
		c:           c,
		robotClient: client,
		logger:      logger,
	}

	vc.bottleTop = referenceframe.NewLinkInFrame(
		vc.conf.BottleGripper,
		spatialmath.NewPose(r3.Vector{vc.conf.BottleHeight - 70, -7, 0}, &spatialmath.OrientationVectorDegrees{OX: 1}),
		bottleName,
		nil,
	)

	vc.pourExtraFrames = []*referenceframe.LinkInFrame{vc.bottleTop}

	var err error
	vc.pourWorldState, err = referenceframe.NewWorldState(nil, vc.pourExtraFrames)
	if err != nil {
		return nil, err
	}

	err = vc.setupPourPositions(ctx)
	if err != nil {
		return nil, err
	}

	return vc, nil
}

type VinoCart struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	conf   *Config

	robotClient robot.Robot

	c *Pour1Components

	bottleTop       *referenceframe.LinkInFrame
	pourExtraFrames []*referenceframe.LinkInFrame
	pourWorldState  *referenceframe.WorldState

	pourJoints [][]referenceframe.Input
	pourPoses  []*referenceframe.PoseInFrame
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

func (vc *VinoCart) Reset(ctx context.Context) error {
	err := vc.doAll(ctx, "touch", "prep", 100)
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

	return nil
}

func (vc *VinoCart) GrabCup(ctx context.Context) error {
	got, err := vc.c.Gripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	if !got {
		return fmt.Errorf("didn't get cup")
	}

	return nil
}

func (vc *VinoCart) Touch(ctx context.Context) error {
	vc.logger.Infof("touch called")

	err := vc.Reset(ctx)
	if err != nil {
		return err
	}

	if vc.conf.SimoneHack {
		err = vc.doAll(ctx, "touch", "pickup-hack", 60)
		if err != nil {
			return err
		}

		return vc.GrabCup(ctx)
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

	return vc.GrabCup(ctx)
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

func (vc *VinoCart) getPositions(stage, step string) ([][]toggleswitch.Switch, error) {
	steps, ok := vc.c.Positions[stage]
	if !ok {
		return nil, fmt.Errorf("no stage %s", stage)
	}
	positions, ok := steps[step]
	if !ok {
		return nil, fmt.Errorf("no step [%s] in stage [%s]", step, stage)
	}
	return positions, nil
}

func (vc *VinoCart) doAll(ctx context.Context, stage, step string, speedAndAccelBothArm float64) error {
	err := SetXarmSpeed(ctx, vc.c.Arm, speedAndAccelBothArm, speedAndAccelBothArm)
	if err != nil {
		return err
	}

	err = SetXarmSpeed(ctx, vc.c.BottleArm, speedAndAccelBothArm, speedAndAccelBothArm)
	if err != nil {
		return err
	}

	positions, err := vc.getPositions(stage, step)
	if err != nil {
		return err
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
	err := vc.doAll(ctx, "pour_prep", "prep-grab", 80)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "pour_prep", "right-grab", 80)
	if err != nil {
		return err
	}

	err = vc.pourPrepGrab(ctx)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "pour_prep", "post-grab", 50)
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
	err := vc.doAll(ctx, "pour", "prep", 50)
	if err != nil {
		return err
	}

	time.Sleep(500 * time.Millisecond)

	start := time.Now()
	loopNumber := 0

	var pd *pourDetector

	totalTime := 15 * time.Second
	markedDifferent := false

	pourContext, cancelPour := context.WithCancel(ctx)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := vc.doPourMotion(ctx, pourContext)
		if err != nil {
			vc.logger.Infof("error pouring: %v", err)
		}
	}()

	defer func() {
		cancelPour()
		wg.Wait() // this goes back down

		SetXarmSpeedLog(ctx, vc.c.BottleArm, 50, 50, vc.logger)

		err := vc.doAll(ctx, "pour", "finish", 50)
		if err != nil {
			vc.logger.Infof("error in pour cleanup: %v", err)
		}
	}()

	for time.Since(start) < totalTime {
		loopStart := time.Now()

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

	// cleanup done in defer above
	return nil
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
	err := vc.doAll(ctx, "put-back", "before-open", 50)
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

	return vc.doAll(ctx, "put-back", "post-open", 100)
}

func (vc *VinoCart) PourMotionDemo(ctx context.Context) error {
	err := SetXarmSpeed(ctx, vc.c.BottleArm, 75, 75)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "pour", "prep", 75)
	if err != nil {
		return err
	}

	pourContext, cancel := context.WithCancel(ctx)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := vc.doPourMotion(ctx, pourContext)
		if err != nil {
			vc.logger.Infof("eliot: %v", err)
		}
	}()

	time.Sleep(2 * time.Second)

	vc.logger.Infof("cancel called")
	cancel()

	wg.Wait()
	vc.logger.Infof("wait done")

	return nil
}

func (vc *VinoCart) doPourMotion(ctx, pourContext context.Context) error {
	err := SetXarmSpeed(ctx, vc.c.BottleArm, 20, 50)
	if err != nil {
		return err
	}
	defer SetXarmSpeedLog(ctx, vc.c.BottleArm, 50, 50, vc.logger)

	err = vc.c.BottleArm.MoveThroughJointPositions(pourContext, vc.pourJoints, nil, nil)

	if err != nil && err != context.Canceled && pourContext.Err() != context.Canceled {
		return err
	}

	vc.logger.Infof("going back down")

	cur, err := vc.c.BottleMotionService.GetPose(ctx, resource.Name{Name: bottleName}, "world", vc.pourExtraFrames, nil)
	if err != nil {
		return err
	}

	err = SetXarmSpeed(ctx, vc.c.BottleArm, 75, 75)
	if err != nil {
		return err
	}

	posesToDo := [][]referenceframe.Input{}

	for i := len(vc.pourPoses) - 1; i >= 0; i-- {
		if cur.Pose().Orientation().OrientationVectorDegrees().OZ > vc.pourPoses[i].Pose().Orientation().OrientationVectorDegrees().OZ {
			continue
		}

		posesToDo = append(posesToDo, vc.pourJoints[i])
	}

	return vc.c.BottleArm.MoveThroughJointPositions(ctx, posesToDo, nil, nil)
}

func (vc *VinoCart) setupPourPositions(ctx context.Context) error {

	allPostions, err := vc.getPositions("pour", "prep")
	if err != nil {
		return err
	}

	pos := allPostions[len(allPostions)-1][0] // HACK HACK HACK

	res, err := pos.DoCommand(ctx, map[string]interface{}{"cfg": true})
	if err != nil {
		return fmt.Errorf("couldn't get cfg from pos0 step %w", err)
	}

	posConfig := touch.ArmPositionSaverConfig{}

	err = json.Unmarshal([]byte(res["as_json"].(string)), &posConfig)
	if err != nil {
		vc.logger.Errorf("bad json %v", res)
		return err
	}

	if posConfig.Point.X == 0 {
		return fmt.Errorf("config likely broken %v", posConfig)
	}

	armPose := spatialmath.NewPose(posConfig.Point, &posConfig.Orientation)
	vc.logger.Infof("armPose: %v", armPose)

	fsc, err := vc.robotClient.FrameSystemConfig(ctx)
	if err != nil {
		return err
	}

	x := touch.FindPart(fsc, vc.conf.BottleGripper)
	if x == nil {
		return fmt.Errorf("can't find frame for BottleGripper %v", vc.conf.BottleGripper)
	}

	if x.FrameConfig.Parent() != posConfig.Arm {
		return fmt.Errorf("parent wrong %v %v", x.FrameConfig.Parent(), posConfig.Arm)
	}

	vc.logger.Infof("pp: %v", x.FrameConfig.Pose())

	gripperPose := spatialmath.Compose(armPose, x.FrameConfig.Pose())
	vc.logger.Infof("gripperPose: %v", gripperPose)

	bottleStart := spatialmath.Compose(gripperPose, vc.bottleTop.Pose())
	vc.logger.Infof("bottleStart: %v", bottleStart)

	o := bottleStart.Orientation().OrientationVectorDegrees()

	joints := [][]referenceframe.Input{}
	poses := []*referenceframe.PoseInFrame{}

	for o.OZ > -.5 {
		goalPose := referenceframe.NewPoseInFrame("world",
			spatialmath.NewPose(
				bottleStart.Point(),
				o,
			),
		)

		poses = append(poses, goalPose)

		vc.logger.Infof(" next: %v", goalPose.Pose())

		hashKey := fmt.Sprintf("foo-%d", int(o.OZ*1000))

		_, err = vc.c.BottleMotionService.Move(
			ctx,
			motion.MoveReq{
				ComponentName: resource.Name{Name: bottleName},
				Destination:   goalPose,
				WorldState:    vc.pourWorldState,
				Extra:         map[string]interface{}{"hash": true, "hash_key": hashKey},
			},
		)
		if err != nil {
			return err
		}

		res, err := vc.c.BottleMotionService.DoCommand(ctx, map[string]interface{}{"get_hash": true, "hash_key": hashKey})
		if err != nil {
			return err
		}

		planRaw, ok := res["plan"].([]interface{})
		if !ok {
			return fmt.Errorf("hack plan not an array %v %T", res, res["plan"])
		}

		for idx, fRaw := range planRaw {
			f2, ok := fRaw.([]interface{})
			if !ok {
				return fmt.Errorf("fRaw %v %T", fRaw, fRaw)
			}

			f := []float64{}
			for _, ff := range f2 {
				fff, ok := ff.(float64)
				if !ok {
					return fmt.Errorf("really now %v %T", f2, f2)
				}
				f = append(f, fff)
			}

			if idx == len(planRaw)-1 {
				joints = append(joints, referenceframe.FloatsToInputs(f))
			}
		}

		o.OZ -= .05
	}

	if len(joints) != len(poses) {
		panic("Wtf")
	}

	vc.pourJoints = joints
	vc.pourPoses = poses
	return nil
}
