// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"

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
	"go.viam.com/rdk/spatialmath"
	rutils "go.viam.com/rdk/utils"
)

var GenericServiceName = resource.NewModel("viam", "viam-pouring-demo", "pour")

func init() {
	resource.RegisterService(generic.API, GenericServiceName, resource.Registration[resource.Resource, *Config]{Constructor: newPour})
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
	return []string{cfg.ArmName, cfg.CameraName, cfg.WeightSensorName, motion.Named("builtin").String()}, nil
}

type Config struct {
	ArmName          string `json:"arm_name"`
	CameraName       string `json:"camera_name"`
	WeightSensorName string `json:"weight_sensor_name"`
	Address          string `json:"address"`
	Entity           string `json:"entity"`
	Payload          string `json:"payload"`
}

// gen is a fake Generic service that always echos input back to the caller.
type gen struct {
	resource.Resource
	resource.Named
	resource.TriviallyReconfigurable
	resource.TriviallyCloseable
	logger      logging.Logger
	address     string
	entity      string
	payload     string
	robotClient *client.RobotClient
	a           arm.Arm
	c           camera.Camera
	s           sensor.Sensor
	m           motion.Service
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
	// here I want to validate that I actually have access the weight-sensor, arm, and camera
	endPos, err := g.a.EndPosition(ctx, nil)
	if err != nil {
		return nil, err
	}
	g.logger.Infof("endPos: %v", spatialmath.PoseToProtobuf(endPos))

	props, err := g.c.Properties(ctx)
	if err != nil {
		return nil, err
	}
	g.logger.Infof("props: %v", props)

	readings, err := g.s.Readings(ctx, nil)
	if err != nil {
		return nil, err
	}
	g.logger.Infof("readings: %v", readings)

	fsCfg, err := g.robotClient.FrameSystemConfig(ctx)
	if err != nil {
		return nil, err
	}
	g.logger.Infof("fsCfg: %v", fsCfg)

	// g.calibrate()

	return cmd, nil
}
