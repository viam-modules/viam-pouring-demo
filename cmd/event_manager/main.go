package main

import (
	"context"
	"errors"
	"sync"

	goutils "go.viam.com/utils"

	"go.viam.com/rdk/components/generic"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
)

var Model = resource.NewModel("ncs", "winedemo", "event_manager")

const (
	fullRedBottleWeight   = 1500
	fullWhiteBottleWeight = 1500
	pourWeight            = 150
)

type eventManager struct {
	mu   sync.Mutex
	name resource.Name
	resource.TriviallyCloseable
	resource.TriviallyReconfigurable
	redBottleWeight   float64
	whiteBottleWeight float64
	logger            logging.Logger
}

type Config struct {
	Names []string `json:"names,omitempty"`
}

func (c *Config) Validate(path string) ([]string, error) {
	return nil, nil
}

func (f *eventManager) DoCommand(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logger.Infof("%#v\n", extra)
	cmd, ok := extra["command"].(string)
	if !ok {
		return nil, errors.New("invalid command type")
	}

	color, ok := extra["color"].(string)
	if !ok {
		return nil, errors.New("provide color")
	}

	if color != "red" && color != "white" {
		return nil, errors.New("color must be white or red")
	}

	switch cmd {
	case "get_weight_remaining":
		if color == "red" {
			return map[string]interface{}{"weight_remaining": f.redBottleWeight}, nil
		}

		if color == "white" {
			return map[string]interface{}{"weight_remaining": f.whiteBottleWeight}, nil
		}
	case "set_new_bottle":
		if color == "red" {
			f.redBottleWeight = fullRedBottleWeight
		}

		if color == "white" {
			f.whiteBottleWeight = fullWhiteBottleWeight
		}
		return nil, nil

	case "set_glasses_poured":
		numGlassesStr, ok := extra["num_glasses"]
		if !ok {
			return nil, errors.New("provide num_glasses")
		}
		f.logger.Infof("numGlasses type: %T", numGlassesStr)

		numGlasses, ok := numGlassesStr.(float64)
		if !ok {
			return nil, errors.New("provide num_glasses as a float")
		}

		if color == "red" {
			f.redBottleWeight = f.redBottleWeight - (pourWeight * float64(numGlasses))
		}

		if color == "white" {
			f.whiteBottleWeight = f.whiteBottleWeight - (pourWeight * float64(numGlasses))
		}
		return nil, nil

	default:
		return nil, errors.New("unsupported command")
	}
	return nil, errors.New("unsupported command")
}

func (s *eventManager) Name() resource.Name {
	return s.name
}

func newEventManager(_ context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (resource.Resource, error) {
	return &eventManager{logger: logger}, nil
}

func mainWithArgs(ctx context.Context, args []string, logger logging.Logger) (err error) {
	resource.RegisterComponent(
		generic.API,
		Model,
		resource.Registration[resource.Resource, *Config]{Constructor: newEventManager})

	module, err := module.NewModuleFromArgs(ctx)
	if err != nil {
		return err
	}
	if err := module.AddModelFromRegistry(ctx, generic.API, Model); err != nil {
		return err
	}

	err = module.Start(ctx)
	defer module.Close(ctx)
	if err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

func main() {
	goutils.ContextualMain(mainWithArgs, module.NewLoggerFromArgs(Model.String()))
}
