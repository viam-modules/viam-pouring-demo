package pour

import (
	"context"
	"fmt"
	"math"
	"time"

	"gonum.org/v1/gonum/stat"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

var WeightModel = NamespaceFamily.WithModel("pouring-weight-smoother")

type WeightConfig struct {
	Scale string
}

func (c WeightConfig) Validate(p string) ([]string, error) {
	if c.Scale == "" {
		return nil, fmt.Errorf("need a scale")
	}

	return []string{c.Scale}, nil
}

func init() {
	resource.RegisterComponent(
		sensor.API,
		WeightModel,
		resource.Registration[sensor.Sensor, *WeightConfig]{
			Constructor: newWeightSensor,
		})
}

func newWeightSensor(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (sensor.Sensor, error) {
	config, err := resource.NativeConfig[*WeightConfig](conf)
	if err != nil {
		return nil, err
	}

	s, err := sensor.FromDependencies(deps, config.Scale)
	if err != nil {
		return nil, err
	}

	return NewWeightSmoother(ctx, conf.ResourceName(), s, logger)
}

func NewWeightSmoother(ctx context.Context, name resource.Name, scale sensor.Sensor, logger logging.Logger) (*WeightSmoother, error) {
	return &WeightSmoother{
		name:   name,
		logger: logger,
		scale:  scale,
	}, nil
}

type WeightSmoother struct {
	resource.TriviallyCloseable
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger

	scale sensor.Sensor
}

func (ws *WeightSmoother) Name() resource.Name {
	return ws.name
}

func (ws *WeightSmoother) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	cycles, ok := vmodutils.GetIntFromMap(extra, "cycles")
	if !ok {
		cycles = 10
	}

	sleep, ok := vmodutils.GetIntFromMap(extra, "sleep")
	if !ok {
		sleep = 25
	}

	field, ok := extra["field"].(string)
	if !ok || field == "" {
		field = "mass_kg"
	}

	res, err := ws.Go(ctx, cycles, sleep, field)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{field: res}, nil
}

func (ws *WeightSmoother) Go(ctx context.Context, cycles, sleep int, field string) (float64, error) {
	all := []float64{}
	for i := 0; i < cycles; i++ {
		x, err := ws.scale.Readings(ctx, nil)
		if err != nil {
			return 0, err
		}
		v, ok := x[field].(float64)
		if !ok {
			return 0, fmt.Errorf("field [%s] was not a float64, was (%v) a %T", field, x[field], x[field])
		}

		ws.logger.Debugf("got raw reading of %v", v)

		all = append(all, v)
		time.Sleep(time.Millisecond * time.Duration(sleep))
	}

	res := getBestNumberForWeight(all)

	ws.logger.Debugf("smoother to reading of %v", res)

	return res, nil
}

func (ws *WeightSmoother) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}

func getBestNumberForWeight(raw []float64) float64 {

	if len(raw) == 0 {
		return 0
	}

	if len(raw) == 1 {
		return raw[0]
	}

	mean := stat.Mean(raw, nil)
	stdDev := math.Sqrt(stat.Variance(raw, nil))

	good := []float64{}
	for _, v := range raw {
		if math.Abs(mean-v) <= stdDev {
			good = append(good, v)
		}
	}

	return stat.Mean(good, nil)
}
