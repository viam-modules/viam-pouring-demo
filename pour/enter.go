// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"fmt"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
)

var Name = resource.NewModel("viam", "motion", "pour")

func init() {
	resource.RegisterService(generic.API, Name, resource.Registration[*gen, *Config]{
		Constructor: newPour,
	})
}

func newPour(
	ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger,
) (*gen, error) {
	g := &gen{}
	g.Reconfigure(ctx, deps, conf)
	return g, nil

}

func (cfg *Config) Validate(path string) ([]string, error) {
	return []string{}, nil
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
	logger logging.Logger
	a      arm.Arm
	c      camera.Camera
	s      sensor.Sensor
	m      motion.Service
}

// DoCommand echos input back to the caller.
func (g *gen) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	// here I want to validate that I actually have access the weight-sensor, arm, and camera
	endPos, err := g.a.EndPosition(ctx, nil)
	if err != nil {
		return nil, err
	}
	fmt.Println("endPos: ", spatialmath.PoseToProtobuf(endPos))

	props, err := g.c.Properties(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Println("props: ", props)

	readings, err := g.s.Readings(ctx, nil)
	if err != nil {
		return nil, err
	}
	fmt.Println("readings: ", readings)

	return cmd, nil
}

func (g *gen) Name() resource.Name {
	return resource.NewName(generic.API, "liquid-pouring-demo")
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

	return nil
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
}
