// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"

	"go.uber.org/multierr"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/utils/rpc"
)

var Model = NamespaceFamily.WithModel("pour")

func init() {
	resource.RegisterService(generic.API, Model, resource.Registration[resource.Resource, *Config]{Constructor: newPour})
}

func newPour(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	config, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
	}

	g := &Gen{
		name:   conf.ResourceName(),
		logger: logger,
		conf:   config,
	}

	g.arm, err = arm.FromDependencies(deps, config.ArmName)
	if err != nil {
		return nil, err
	}

	g.gripper, err = gripper.FromDependencies(deps, config.GripperName)
	if err != nil {
		return nil, err
	}

	g.cam, err = camera.FromDependencies(deps, config.CameraName)
	if err != nil {
		return nil, err
	}

	g.weight, err = sensor.FromDependencies(deps, config.WeightSensorName)
	if err != nil {
		return nil, err
	}

	g.motion, err = motion.FromDependencies(deps, "builtin")
	if err != nil {
		return nil, err
	}

	g.camVision, err = vision.FromDependencies(deps, config.CircleDetectionService)
	if err != nil {
		return nil, err
	}

	err = g.setupRobotClient(ctx)
	if err != nil {
		return nil, err
	}

	g.web, err = createAndRunWebServer(g, 8888, logger)
	if err != nil {
		return nil, err
	}

	g.numThreads = config.CPUThreads
	if g.numThreads == 0 {
		g.numThreads = runtime.NumCPU() / 2
	}

	logger.Info("the pouring module has been constructed")
	return g, nil
}

func (cfg *Config) Validate(path string) ([]string, error) {
	if cfg.ArmName == "" {
		return nil, fmt.Errorf("need an arm name")
	}

	if cfg.GripperName == "" {
		return nil, fmt.Errorf("need a gripper name")
	}
	if cfg.CameraName == "" {
		return nil, fmt.Errorf("need a camera name")
	}
	if cfg.WeightSensorName == "" {
		return nil, fmt.Errorf("need a weight name")
	}
	if cfg.CircleDetectionService == "" {
		return nil, fmt.Errorf("need a circledetectionservice name")
	}

	return []string{cfg.ArmName, cfg.GripperName, cfg.CameraName, cfg.WeightSensorName, motion.Named("builtin").String(), cfg.CircleDetectionService}, nil
}

type Config struct {
	ArmName                string `json:"arm_name"`
	CameraName             string `json:"camera_name"`
	CircleDetectionService string `json:"circle_detection_service"`
	WeightSensorName       string `json:"weight_sensor_name"`
	GripperName            string `json:"gripper_name"`

	BottleHeight float64 `json:"bottle_height"`
	CupHeight    float64 `json:"cup_height"`

	CPUThreads int `json:"cpu_threads"`
}

func NewTesting(logger logging.Logger,
	client *client.RobotClient,
	arm arm.Arm,
	gripper gripper.Gripper,
	cam camera.Camera,
	weight sensor.Sensor,
	motion motion.Service,
	camVision vision.Service,
) *Gen {
	return &Gen{
		robotClient: client,
		arm:         arm,
		gripper:     gripper,
		cam:         cam,
		weight:      weight,
		motion:      motion,
		camVision:   camVision,
		logger:      logger,
		conf:        &Config{},
	}
}

type Gen struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	conf   *Config

	address string
	entity  string
	payload string

	web *http.Server

	robotClient *client.RobotClient

	arm       arm.Arm
	gripper   gripper.Gripper
	cam       camera.Camera
	weight    sensor.Sensor
	motion    motion.Service
	camVision vision.Service

	statusLock sync.Mutex
	status     string

	numThreads int
}

func (g *Gen) setupRobotClient(ctx context.Context) error {
	vc, err := app.CreateViamClientFromEnvVars(ctx, nil, g.logger)
	if err != nil {
		return err
	}
	defer vc.Close()

	g.payload = os.Getenv("VIAM_API_KEY")
	g.entity = os.Getenv("VIAM_API_KEY_ID")

	r, _, err := vc.AppClient().GetRobotPart(ctx, os.Getenv("VIAM_MACHINE_PART_ID"))
	if err != nil {
		return err
	}

	g.address = r.FQDN

	machine, err := client.New(
		ctx,
		g.address,
		g.logger,
		client.WithDialOptions(rpc.WithEntityCredentials(
			g.entity,
			rpc.Credentials{
				Type:    rpc.CredentialsTypeAPIKey,
				Payload: g.payload,
			})),
	)
	if err != nil {
		return err
	}
	g.robotClient = machine
	return nil
}

func (g *Gen) Name() resource.Name {
	return g.name
}

func (g *Gen) Close(ctx context.Context) error {
	return multierr.Combine(
		g.robotClient.Close(ctx),
		g.web.Close(),
	)
}

// DoCommand
func (g *Gen) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	// TODO-eliot cancel old movement

	g.logger.Infof("cmd: %v", cmd)
	if _, ok := cmd["stop"]; ok {
		g.logger.Info("WE ARE INSIDE THE STOP CONDITIONAL AND ARE ABOUT TO RETURN")
		return nil, g.arm.Stop(ctx, nil)
	}

	if _, ok := cmd["status"]; ok {
		return map[string]interface{}{"status": g.getStatus()}, nil
	}

	if _, ok := cmd["reset"]; ok {
		err := g.ResetArmToHome(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"reset": true}, nil
	}

	doPour, _ := cmd["do-pour"].(bool)

	err := g.startPouringProcess(ctx, doPour)
	if err != nil {
		g.setStatus(fmt.Sprintf("error: %v", err))
	} else {
		g.setStatus("success")
	}

	return map[string]interface{}{}, err
}

func (g *Gen) setStatus(input string) {
	g.logger.Info(input)
	g.statusLock.Lock()
	defer g.statusLock.Unlock()
	g.status = input
}

func (g *Gen) getStatus() string {
	g.statusLock.Lock()
	defer g.statusLock.Unlock()
	return g.status
}

func (g *Gen) ResetArmToHome(ctx context.Context) error {

	err := g.arm.MoveToJointPositions(ctx, JointPositionsPreppingForPour, nil)
	if err != nil {
		return err
	}

	err = g.arm.MoveToJointPositions(ctx, JointPositionsPickUp, nil)
	if err != nil {
		return err
	}

	return g.gripper.Open(ctx, nil)
}
