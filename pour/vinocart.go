package pour

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.uber.org/multierr"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/components/switch"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
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

//go:embed vinoweb/dist
var vinowebStaticFS embed.FS

const bottleName = "bottle-top"
const gripperToCupCenterHack = -10

var VinoCartModel = NamespaceFamily.WithModel("vinocart")
var noObjects = fmt.Errorf("no objects")

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

	g, err := NewVinoCart(ctx, config, c, robotClient, nil, logger)
	if err != nil {
		return nil, err
	}

	logger.Info("the pouring module has been constructed")
	return g, nil
}

func NewVinoCart(ctx context.Context, conf *Config, c *Pour1Components, client robot.Robot, dataClient *app.DataClient, logger logging.Logger) (*VinoCart, error) {
	vc := &VinoCart{
		conf:        conf,
		c:           c,
		robotClient: client,
		dataClient:  dataClient,
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

	if conf.Loop {
		vc.status = "starting"
		vc.loopWaitGroup.Add(1)
		cancelCtx, cancel := context.WithCancel(context.Background())
		vc.loopCancel = cancel
		go vc.run(cancelCtx)
	} else {
		vc.status = "manual mode"
	}

	realFS, err := fs.Sub(vinowebStaticFS, "vinoweb/dist")
	if err != nil {
		return nil, err
	}

	_, vc.server, err = vmodutils.PrepInModuleServer(realFS, logger.Sublogger("accesslog"))
	if err != nil {
		return nil, err
	}
	go func() {
		vc.server.Addr = ":9999"
		vc.logger.Infof("web listening on %s", vc.server.Addr)
		vc.server.ListenAndServe()
	}()

	return vc, nil
}

type VinoCart struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	conf   *Config

	robotClient robot.Robot
	dataClient  *app.DataClient

	c *Pour1Components

	bottleTop       *referenceframe.LinkInFrame
	pourExtraFrames []*referenceframe.LinkInFrame
	pourWorldState  *referenceframe.WorldState

	pourJoints [][]referenceframe.Input
	pourPoses  []*referenceframe.PoseInFrame

	loopCancel    context.CancelFunc
	loopWaitGroup sync.WaitGroup

	statusLock sync.Mutex
	status     string

	server *http.Server
}

func (vc *VinoCart) Name() resource.Name {
	return vc.name
}

func (vc *VinoCart) Close(ctx context.Context) error {
	if vc.loopCancel != nil {
		vc.loopCancel()
		vc.loopWaitGroup.Wait()
	}

	return multierr.Combine(vc.robotClient.Close(ctx), vc.server.Close())
}

func (vc *VinoCart) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	if cmd["status"] == true {
		return map[string]interface{}{"status": vc.getStatus()}, nil
	}

	if vc.loopCancel != nil {
		return nil, fmt.Errorf("in loop mode, can't do anything but get status")
	}

	defer func() {
		vc.setStatus("manual mode")
	}()

	if cmd["reset"] == true {
		return nil, vc.Reset(ctx)
	}

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

func (vc *VinoCart) run(ctx context.Context) {
	defer vc.loopWaitGroup.Done()
	for ctx.Err() == nil {
		err := vc.WaitForCupAndGo(ctx)
		if err != nil {
			vc.logger.Errorf("go error in run: %v", err)
		}
	}
}

func (vc *VinoCart) getStatus() string {
	vc.statusLock.Lock()
	defer vc.statusLock.Unlock()
	return vc.status
}

func (vc *VinoCart) setStatus(s string) {
	vc.statusLock.Lock()
	defer vc.statusLock.Unlock()
	vc.status = s
}

func (vc *VinoCart) WaitForCupAndGo(ctx context.Context) error {
	for {
		vc.setStatus("standby")
		err := vc.FullDemo(ctx)
		if err == nil {
			break
		}
		if err != noObjects {
			return err
		}
		vc.logger.Infof("got %v, looping", err)
	}

	vc.setStatus("waiting")

	// need to wait till the area is clear
	vc.logger.Infof("waiting for area to be clear")

	for {
		objects, err := vc.FindCups(ctx)
		if err != nil {
			return err
		}

		vc.logger.Infof("num objects while waiting: %v", len(objects))
		for _, o := range objects {
			vc.logger.Infof("\t objects: %v", o)
		}
		if len(objects) == 0 {
			return nil
		}
	}
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
	g := errgroup.Group{}

	g.Go(func() error {
		return vc.c.Gripper.Open(ctx, nil)
	})

	g.Go(func() error {
		return vc.c.BottleGripper.Open(ctx, nil)
	})

	err := vc.doAll(ctx, "touch", "prep", 100)
	err2 := g.Wait()

	return multierr.Combine(err, err2)
}

func (vc *VinoCart) GrabCup(ctx context.Context) error {
	got, err := vc.c.Gripper.Grab(ctx, nil)
	if err != nil {
		return err
	}

	if !got {
		return fmt.Errorf("didn't get cup")
	}

	return vc.checkPickQuality(ctx)
}

func (vc *VinoCart) checkPickQuality(ctx context.Context) error {

	if vc.c.PickQualityService == nil {
		return nil
	}

	imgs, _, err := vc.c.Cam.Images(ctx)
	if err != nil {
		return err
	}

	if imgs[0].SourceName != "" && imgs[0].SourceName != "color" {
		return fmt.Errorf("bad image name [%v]", imgs[0].SourceName)
	}

	prepped, err := prepPickImage(imgs[0].Image)
	if err != nil {
		return err
	}

	if vc.dataClient != nil {
		err := vc.saveCupImage(ctx, prepped)
		if err != nil {
			vc.logger.Warnf("can't saveCupImage: %v", err)
		}
	}

	cs, err := vc.c.PickQualityService.Classifications(ctx, prepped, 1, nil)
	if err != nil {
		return err
	}

	vc.logger.Infof("quality: %v", cs)

	if len(cs) != 1 {
		return fmt.Errorf("why is quality array wrong: %v", cs)
	}

	if cs[0].Label() == "good" {
		return nil
	}

	// bad pick, move

	err = vc.doAll(ctx, "touch", "bad-pick-a", 50)
	if err != nil {
		return err
	}

	err = vc.c.Gripper.Open(ctx, nil)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "touch", "bad-pick-b", 50)
	if err != nil {
		return err
	}

	return fmt.Errorf("bad pick %v", cs[0])
}

func (vc *VinoCart) saveCupImage(ctx context.Context, prepped image.Image) error {
	if vc.dataClient == nil {
		return fmt.Errorf("no data client")
	}

	pid := os.Getenv("VIAM_MACHINE_PART_ID")
	if pid == "" {
		return fmt.Errorf("VIAM_MACHINE_PART_ID not defined")
	}

	data, err := encodePNG(prepped)
	if err != nil {
		return err
	}

	ct := "rdk:component:camera"
	fn := fmt.Sprintf("cup-pick-edges-%d.png", time.Now().Unix())
	pngString := "png"

	opts := app.FileUploadOptions{
		ComponentType: &ct,
		ComponentName: &vc.conf.CameraName,
		FileName:      &fn,
		FileExtension: &pngString,
	}

	id, err := vc.dataClient.FileUploadFromBytes(ctx, os.Getenv("VIAM_MACHINE_PART_ID"), data, &opts)
	if err != nil {
		return err
	}

	err = vc.dataClient.AddBinaryDataToDatasetByIDs(ctx, []string{id}, "683f8952383a821481d9b5c9")
	if err != nil {
		return err
	}

	vc.logger.Infof("uploaded image to dataset for cup pick quality")

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

	start := time.Now()
	objects, err := vc.FindCups(ctx)
	if err != nil {
		return err
	}

	vc.logger.Infof("num objects: %v in %v", len(objects), time.Since(start))
	for _, o := range objects {
		vc.logger.Infof("\t objects: %v", o)
	}

	if len(objects) == 0 {
		return noObjects
	}

	if len(objects) > 1 {
		return fmt.Errorf("too many objects %d", len(objects))
	}

	vc.setStatus("picking")

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
		{OY: 1, Theta: 180},
		{OX: .5, OY: 1, Theta: 180},
		{OX: 1, OY: 1, Theta: 180},
		{OX: 1, OY: -1, Theta: 180},
		{OY: -1, Theta: 180},
		{OX: -.5, OY: -1, Theta: 180},
	}

	approaches := []*referenceframe.PoseInFrame{}

	for _, tryO := range choices {
		goToPose := vc.getApproachPoint(obj, 100, tryO)
		approaches = append(approaches, goToPose)
		vc.logger.Infof("trying to move to %v", goToPose.Pose())

		_, err2 := vc.c.CupMotionService.Move(
			ctx,
			motion.MoveReq{
				ComponentName: resource.Name{Name: vc.c.Gripper.Name().ShortName()},
				Destination:   goToPose,
				WorldState:    worldState,
			},
		)

		if err2 != nil {
			vc.logger.Debugf("error: %v", err2)
		}

		if err2 == nil {
			err = nil
			o = tryO
			break
		} else if err == nil {
			err = err2
		}

	}

	if vc.conf.Handoff && err != nil {

		err2 := vc.handoffCupBottleToCupArm(ctx, worldState, approaches, choices, obj)
		if err2 == nil {
			return nil
		}

		return multierr.Combine(err, err2)
	}

	if err != nil {
		return err
	}

	// ---- go to pick up
	err = SetXarmSpeed(ctx, vc.c.Arm, 25, 25)
	if err != nil {
		return err
	}

	goToPose := vc.getApproachPoint(obj, gripperToCupCenterHack, o)
	vc.logger.Infof("going to move to %v", goToPose)

	err = moveWithLinearConstraint(ctx, vc.c.CupMotionService, vc.c.Gripper.Name(), goToPose)
	if err != nil {
		return err
	}

	return vc.GrabCup(ctx)
}

func (vc *VinoCart) handoffCupBottleToCupArm(ctx context.Context, worldState *referenceframe.WorldState, approaches []*referenceframe.PoseInFrame, choices []*spatialmath.OrientationVectorDegrees, obj *viz.Object) error {
	for idx, goToPose := range approaches {
		vc.logger.Infof("trying to move (2) to %v", goToPose.Pose())

		_, err := vc.c.Motion.Move(
			ctx,
			motion.MoveReq{
				ComponentName: resource.Name{Name: vc.c.BottleGripper.Name().ShortName()},
				Destination:   goToPose,
				WorldState:    worldState,
			},
		)
		if err != nil {
			vc.logger.Infof("error (2): %v", err)
			continue
		}

		// we found a path!

		goToPose = vc.getApproachPoint(obj, gripperToCupCenterHack, choices[idx])
		vc.logger.Infof("going to move (2) to %v", goToPose)

		err = moveWithLinearConstraint(ctx, vc.c.Motion, vc.c.BottleGripper.Name(), goToPose)
		if err != nil {
			return err
		}

		got, err := vc.c.BottleGripper.Grab(ctx, nil)
		if err != nil {
			return err
		}
		if !got {
			return fmt.Errorf("didn't grab cup from bottlegripper")
		}

		// move to known spot
		goToPose = vc.getApproachPoint(obj, 150, choices[idx])
		err = moveWithLinearConstraint(ctx, vc.c.Motion, vc.c.BottleGripper.Name(), goToPose)
		if err != nil {
			return err
		}

		// release
		err = vc.c.BottleGripper.Open(ctx, nil)
		if err != nil {
			return err
		}

		// backup
		goToPose = vc.getApproachPoint(obj, 250, choices[idx])
		err = moveWithLinearConstraint(ctx, vc.c.Motion, vc.c.BottleGripper.Name(), goToPose)
		if err != nil {
			return err
		}

		return vc.Touch(ctx)
	}
	return fmt.Errorf("no path for handoff")
}

func (vc *VinoCart) getApproachPoint(obj *viz.Object, deltaLinear float64, o *spatialmath.OrientationVectorDegrees) *referenceframe.PoseInFrame {
	md := obj.MetaData()
	c := md.Center()

	p := getApproachPoint(md, c, deltaLinear, o, vc.logger)
	p.Z = vc.conf.CupHeight - 25

	return referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(p, o),
	)
}

func getApproachPoint(md pointcloud.MetaData, c r3.Vector, deltaLinear float64, o *spatialmath.OrientationVectorDegrees, logger logging.Logger) r3.Vector {

	d := math.Pow((o.OX*o.OX)+(o.OY*o.OY), .5)

	xLinear := (o.OX * deltaLinear / d)
	yLinear := (o.OY * deltaLinear / d)

	logger.Infof("xLinear: %0.2f yLinear: %0.2f", xLinear, yLinear)

	approachPoint := r3.Vector{
		X: c.X - xLinear,
		Y: c.Y - yLinear,
		Z: c.Z,
	}

	return approachPoint
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

func (vc *VinoCart) DoAll(ctx context.Context, stage, step string) error {
	return vc.doAll(ctx, stage, step, 50)
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

func (vc *VinoCart) PourPrepGrab(ctx context.Context) error {

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
	positions[5].Value -= .3 // tilt bottle to increase friction

	err = vc.c.BottleArm.MoveToJointPositions(ctx, positions, nil)
	if err != nil {
		return err
	}

	return nil
}

func (vc *VinoCart) PourPrep(ctx context.Context) error {
	vc.setStatus("prepping")
	err := vc.doAll(ctx, "pour_prep", "prep-grab", 80)
	if err != nil {
		return err
	}

	err = vc.doAll(ctx, "pour_prep", "right-grab", 80)
	if err != nil {
		return err
	}

	err = vc.PourPrepGrab(ctx)
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
	vc.setStatus("pouring")
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
			deltaMax := vc.conf.glassPourMotionThreshold()
			vc.logger.Infof("fn: %v delta: %0.2f (%f)", fn, delta, deltaMax)
			if delta >= deltaMax && !markedDifferent {
				vc.logger.Infof(" **** motion detected *** ")
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
	vc.setStatus("placing")
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

func moveWithLinearConstraint(ctx context.Context, m motion.Service, n resource.Name, p *referenceframe.PoseInFrame) error {
	_, err := m.Move(
		ctx,
		motion.MoveReq{
			ComponentName: resource.Name{Name: n.ShortName()},
			Destination:   p,
			Constraints:   &LinearConstraint,
		},
	)
	return err
}

func (vc *VinoCart) FindCups(ctx context.Context) ([]*viz.Object, error) {
	objects, err := vc.c.CupFinder.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return nil, err
	}

	return FilterObjects(objects, vc.conf.CupHeight, vc.conf.CupHeight*.6, 25, vc.logger), nil
}
