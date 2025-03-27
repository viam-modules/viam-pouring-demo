// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"net/http"
	"os"
	"sync"

	"go.uber.org/multierr"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/utils/rpc"
)

var Model = resource.NewModel("viam", "pouring-demo", "pour")

func init() {
	resource.RegisterService(generic.API, Model, resource.Registration[resource.Resource, *Config]{Constructor: newPour})
}

func newPour(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	config, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
	}

	g := &gen{
		name:   conf.ResourceName(),
		logger: logger,
		conf:   config,
	}

	g.a, err = arm.FromDependencies(deps, config.ArmName)
	if err != nil {
		return nil, err
	}

	g.c, err = camera.FromDependencies(deps, config.CameraName)
	if err != nil {
		return nil, err
	}

	g.s, err = sensor.FromDependencies(deps, config.WeightSensorName)
	if err != nil {
		return nil, err
	}

	g.m, err = motion.FromDependencies(deps, "builtin")
	if err != nil {
		return nil, err
	}

	g.v, err = vision.FromDependencies(deps, config.CircleDetectionService)
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

	logger.Info("the pouring module has been constructed")
	return g, nil
}

func (cfg *Config) Validate(path string) ([]string, error) {
	return []string{cfg.ArmName, cfg.CameraName, cfg.WeightSensorName, motion.Named("builtin").String(), cfg.CircleDetectionService}, nil
}

type Config struct {
	ArmName                string `json:"arm_name"`
	CameraName             string `json:"camera_name"`
	CircleDetectionService string `json:"circle_detection_service"`
	WeightSensorName       string `json:"weight_sensor_name"`

	DeltaXPos    float64 `json:"delta_x_pos"`
	DeltaYPos    float64 `json:"delta_y_pos"`
	DeltaXNeg    float64 `json:"delta_x_neg"`
	DeltaYNeg    float64 `json:"delta_y_neg"`
	BottleHeight float64 `json:"bottle_height"`
}

type gen struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	conf   *Config

	address string
	entity  string
	payload string

	web *http.Server

	robotClient *client.RobotClient
	a           arm.Arm
	c           camera.Camera
	s           sensor.Sensor
	m           motion.Service
	v           vision.Service

	statusLock sync.Mutex
	status     string
}

func (g *gen) setupRobotClient(ctx context.Context) error {
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

	g.logger.Warnf("hi %#v", r)
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

func (g *gen) Name() resource.Name {
	return g.name
}

func (g *gen) Close(ctx context.Context) error {
	return multierr.Combine(
		g.robotClient.Close(ctx),
		g.web.Close(),
	)
}

// DoCommand echos input back to the caller.
func (g *gen) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	g.logger.Infof("cmd: %v", cmd)
	if _, ok := cmd["stop"]; ok {
		g.logger.Info("WE ARE INSIDE THE STOP CONDITIONAL AND ARE ABOUT TO RETURN")
		return nil, g.a.Stop(ctx, nil)
	}
	if _, ok := cmd["status"]; ok {
		return map[string]interface{}{"status": g.getStatus()}, nil
	}
	return cmd, g.calibrate(ctx)
}

func (g *gen) setStatus(input string) {
	g.statusLock.Lock()
	defer g.statusLock.Unlock()
	g.status = input
}

func (g *gen) getStatus() string {
	g.statusLock.Lock()
	defer g.statusLock.Unlock()
	return g.status
}
