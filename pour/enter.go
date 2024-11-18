// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"sync"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/utils"
	"go.viam.com/utils/rpc"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"

	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"
	rutils "go.viam.com/rdk/utils"
)

var Model = resource.NewModel("viam", "viam-pouring-demo", "pour")

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
		logger:  logger,
		address: address,
		entity:  entity,
		payload: payload,
	}

	if err := g.Reconfigure(ctx, deps, conf); err != nil {
		return nil, err
	}
	logger.Info("the pouring module has been constructed")
	return g, nil
}

func (cfg *Config) Validate(path string) ([]string, error) {
	return []string{cfg.ArmName, cfg.CameraName, cfg.WeightSensorName, motion.Named("builtin").String(), cfg.CircleDetectionService}, nil
}

type Config struct {
	ArmName                string  `json:"arm_name"`
	CameraName             string  `json:"camera_name"`
	CircleDetectionService string  `json:"circle_detection_service"`
	WeightSensorName       string  `json:"weight_sensor_name"`
	Address                string  `json:"address"`
	Entity                 string  `json:"entity"`
	Payload                string  `json:"payload"`
	DeltaXPos              float64 `json:"delta_x_pos"`
	DeltaYPos              float64 `json:"delta_y_pos"`
	DeltaXNeg              float64 `json:"delta_x_neg"`
	DeltaYNeg              float64 `json:"delta_y_neg"`
	BottleHeight           float64 `json:"bottle_height"`
}

// gen is a fake Generic service that always echos input back to the caller.
type gen struct {
	mu sync.Mutex
	resource.Resource
	resource.Named
	resource.TriviallyReconfigurable
	resource.TriviallyCloseable
	logger                                                   logging.Logger
	address, entity, payload                                 string
	robotClient                                              *client.RobotClient
	a                                                        arm.Arm
	c                                                        camera.Camera
	s                                                        sensor.Sensor
	m                                                        motion.Service
	v                                                        vision.Service
	deltaXPos, deltaYPos, deltaXNeg, deltaYNeg, bottleHeight float64
	status                                                   string
}

func (g *gen) Reconfigure(ctx context.Context, deps resource.Dependencies, conf resource.Config) error {
	config, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return err
	}

	a, err := arm.FromDependencies(deps, config.ArmName)
	if err != nil {
		return err
	}
	g.a = a

	c, err := camera.FromDependencies(deps, config.CameraName)
	if err != nil {
		return err
	}
	g.c = c

	s, err := sensor.FromDependencies(deps, config.WeightSensorName)
	if err != nil {
		return err
	}
	g.s = s

	m, err := motion.FromDependencies(deps, "builtin")
	if err != nil {
		return err
	}
	g.m = m

	v, err := vision.FromDependencies(deps, config.CircleDetectionService)
	if err != nil {
		return err
	}
	g.v = v

	g.deltaXPos = config.DeltaXPos
	g.deltaYPos = config.DeltaYPos
	g.deltaXNeg = config.DeltaXNeg
	g.deltaYNeg = config.DeltaYNeg
	g.bottleHeight = config.BottleHeight

	utils.PanicCapturingGo(g.getRobotClient)

	g.logger.Info("done reconfiguring")
	return nil
}

func (g *gen) getRobotClient() {
	machine, err := client.New(
		context.Background(),
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
		g.logger.Fatal(err)
	}
	g.robotClient = machine
}

func (g *gen) Name() resource.Name {
	return resource.NewName(generic.API, "viam_pouring_demo")
}

func (g *gen) Close(ctx context.Context) error {
	return g.robotClient.Close(ctx)
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
	return cmd, g.calibrate()
}

func (g *gen) setStatus(input string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status = input
}

func (g *gen) getStatus() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status
}
