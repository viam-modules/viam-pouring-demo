// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"net/http"
	"sync"

	"go.uber.org/multierr"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/utils/rpc"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"

	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"
	rutils "go.viam.com/rdk/utils"
)

var Model = resource.NewModel("viam", "pouring-demo", "pour")

func init() {
	resource.RegisterService(generic.API, Model, resource.Registration[resource.Resource, *Config]{Constructor: newPour})
}

func newPour(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	address, err := rutils.AssertType[string](conf.Attributes["address"])
	if err != nil {
		return nil, err
	}
	entity, err := rutils.AssertType[string](conf.Attributes["entity"])
	if err != nil {
		return nil, err
	}
	payload, err := rutils.AssertType[string](conf.Attributes["payload"])
	if err != nil {
		return nil, err
	}

	g := &gen{
		name:    conf.ResourceName(),
		logger:  logger,
		address: address,
		entity:  entity,
		payload: payload,
	}

	config, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
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

	g.deltaXPos = config.DeltaXPos
	g.deltaYPos = config.DeltaYPos
	g.deltaXNeg = config.DeltaXNeg
	g.deltaYNeg = config.DeltaYNeg
	g.bottleHeight = config.BottleHeight

	err = g.setupRobotClient(ctx)
	if err != nil {
		return nil, err
	}

	g.web, err = createAndRunWebServer(config, 8888, logger)
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
	Address string `json:"address"`
	Entity  string `json:"entity"`
	Payload string `json:"payload"`

	ArmName                string  `json:"arm_name"`
	CameraName             string  `json:"camera_name"`
	CircleDetectionService string  `json:"circle_detection_service"`
	WeightSensorName       string  `json:"weight_sensor_name"`
	DeltaXPos              float64 `json:"delta_x_pos"`
	DeltaYPos              float64 `json:"delta_y_pos"`
	DeltaXNeg              float64 `json:"delta_x_neg"`
	DeltaYNeg              float64 `json:"delta_y_neg"`
	BottleHeight           float64 `json:"bottle_height"`
}

// gen is a fake Generic service that always echos input back to the caller.
type gen struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger

	web *http.Server

	address, entity, payload                                 string
	robotClient                                              *client.RobotClient
	a                                                        arm.Arm
	c                                                        camera.Camera
	s                                                        sensor.Sensor
	m                                                        motion.Service
	v                                                        vision.Service
	deltaXPos, deltaYPos, deltaXNeg, deltaYNeg, bottleHeight float64

	statusLock sync.Mutex
	status     string
}

func (g *gen) setupRobotClient(ctx context.Context) error {
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
