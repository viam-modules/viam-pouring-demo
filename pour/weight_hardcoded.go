package pour

import (
	"context"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

var WeightHardcodedModel = NamespaceFamily.WithModel("pouring-weight-hardcoded")

type WeightHardcodedConfig struct {
	Weight float64
}

func (c WeightHardcodedConfig) Validate(p string) ([]string, []string, error) {
	return nil, nil, nil
}

func init() {
	resource.RegisterComponent(
		sensor.API,
		WeightHardcodedModel,
		resource.Registration[sensor.Sensor, *WeightHardcodedConfig]{
			Constructor: newWeightHardcoded,
		})
}

func newWeightHardcoded(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (sensor.Sensor, error) {
	config, err := resource.NativeConfig[*WeightHardcodedConfig](conf)
	if err != nil {
		return nil, err
	}

	return NewWeightHardcoded(ctx, conf.ResourceName(), config.Weight, logger)
}

func NewWeightHardcoded(ctx context.Context, name resource.Name, weight float64, logger logging.Logger) (*WeightHardcoded, error) {
	return &WeightHardcoded{
		name:   name,
		logger: logger,
		weight: weight,
	}, nil
}

type WeightHardcoded struct {
	resource.TriviallyCloseable
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger

	weight float64
}

func (ws *WeightHardcoded) Name() resource.Name {
	return ws.name
}

func (ws *WeightHardcoded) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	if extra == nil {
		extra = map[string]interface{}{}
	}

	field, ok := extra["field"].(string)
	if !ok || field == "" {
		field = "mass_kg"
	}

	return map[string]interface{}{field: ws.weight}, nil
}

func (ws *WeightHardcoded) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
