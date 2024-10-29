// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"fmt"
	"runtime/debug"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/robot/framesystem"

	// "go.viam.com/rdk/components/generic"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"

	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
)

var GenericServiceName = resource.NewModel("viam", "viam-pouring-demo", "pour")

func init() {
	fmt.Println("I GET INTO THE INIT FUNCTION")
	resource.RegisterService(generic.API, GenericServiceName, resource.Registration[resource.Resource, *Config]{Constructor: newPour})
	// resource.RegisterService(generic.API, GenericServiceName, resource.Registration[resource.Resource, *Config]{DeprecatedRobotConstructor: newPour})
	// resource.RegisterService(generic.API, GenericServiceName, resource.Registration[resource.Resource, *Config]{
	// 	DeprecatedRobotConstructor: func(
	// 		ctx context.Context, r any, c resource.Config, logger logging.Logger,
	// 	) (resource.Resource, error) {
	// 		actualR, err := utils.AssertType[robot.Robot](r)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		fmt.Println("AM I HERE THOUGH???????")
	// 		return newPour(ctx, actualR, c, logger)
	// 	},
	// })
}

func newPour(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	debug.PrintStack()
	g := &gen{
		logger: logger,
	}
	if err := g.Reconfigure(ctx, deps, conf); err != nil {
		return nil, err
	}
	return g, nil
}

// func newPour(ctx context.Context, robot robot.Robot, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
// 	logger.Info("I GET HERE 1")
// 	g := &gen{
// 		logger: logger,
// 		r:      robot,
// 	}
// 	if err := g.Reconfigure(ctx, nil, conf); err != nil {
// 		return nil, err
// 	}
// 	logger.Info("I GET HERE 2")
// 	return g, nil
// }

func (cfg *Config) Validate(path string) ([]string, error) {
	fmt.Println("I VALIDATED THE CONFIG")
	// return []string{cfg.ArmName, cfg.CameraName, cfg.WeightSensorName, framesystem.InternalServiceName.String()}, nil
	return []string{cfg.ArmName, cfg.CameraName, cfg.WeightSensorName, framesystem.InternalServiceName.String()}, nil
	// return []string{cfg.ArmName, cfg.CameraName, cfg.WeightSensorName, motion.Named("builtin").String()}, nil
}

type Config struct {
	ArmName          string `json:"arm_name"`
	CameraName       string `json:"camera_name"`
	WeightSensorName string `json:"weight_sensor_name"`
}

// gen is a fake Generic service that always echos input back to the caller.
type gen struct {
	resource.Resource
	resource.Named
	resource.TriviallyReconfigurable
	resource.TriviallyCloseable
	logger logging.Logger
	// r      robot.Robot
	a    arm.Arm
	c    camera.Camera
	s    sensor.Sensor
	m    motion.Service
	deps resource.Dependencies
}

// func (g *gen) Reconfigure(ctx context.Context, _ resource.Dependencies, conf resource.Config) error {
// 	config, err := resource.NativeConfig[*Config](conf)
// 	if err != nil {
// 		return err
// 	}

// 	a, err := arm.FromRobot(g.r, config.ArmName)
// 	if err != nil {
// 		return err
// 	}
// 	g.a = a

// 	c, err := camera.FromRobot(g.r, config.CameraName)
// 	if err != nil {
// 		return err
// 	}
// 	g.c = c

// 	s, err := sensor.FromRobot(g.r, config.WeightSensorName)
// 	if err != nil {
// 		return err
// 	}
// 	g.s = s

// 	m, err := motion.FromRobot(g.r, "builtin")
// 	if err != nil {
// 		return err
// 	}
// 	g.m = m

// 	return nil
// }

func (g *gen) Reconfigure(ctx context.Context, deps resource.Dependencies, conf resource.Config) error {
	g.logger.Infof("deps: %v", deps)
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

	fsSvc, err := framesystem.FromDependencies(deps)
	if err != nil {
		g.logger.Infof("THIS IS THE ERROR I HAVE RETURNED222: %v", err)
		return err
	}
	_ = fsSvc

	m, err := motion.FromDependencies(deps, "builtin")
	if err != nil {
		g.logger.Infof("THIS IS THE ERROR I HAVE RETURNED: %v", err)
		return err
	}
	g.m = m

	g.deps = deps

	return nil
}

func (g *gen) Name() resource.Name {
	return g.Name()
}

func (g *gen) Close(ctx context.Context) error {
	if err := g.a.Close(ctx); err != nil {
		return err
	}
	if err := g.c.Close(ctx); err != nil {
		return err
	}
	if err := g.s.Close(ctx); err != nil {
		return err
	}
	if err := g.m.Close(ctx); err != nil {
		return err
	}
	return nil
	// return g.r.Close(ctx)
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

	// g.calibrate()

	return cmd, nil
}
