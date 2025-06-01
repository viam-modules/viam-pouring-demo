// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"

	"go.uber.org/multierr"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/generic"

	"github.com/erh/vmodutils"
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

	g.c, err = Pour1ComponentsFromDependencies(config, deps)
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

func NewTesting(logger logging.Logger, client robot.Robot, c *Pour1Components) *Gen {
	return &Gen{
		robotClient: client,
		c:           c,
		logger:      logger,
		conf: &Config{
			BottleHeight: 310,
			CupHeight:    120,
		},
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

	robotClient robot.Robot

	c *Pour1Components

	statusLock sync.Mutex
	status     string

	numThreads int
}

func (g *Gen) setupRobotClient(ctx context.Context) error {
	machine, err := vmodutils.ConnectToMachineFromEnv(ctx, g.logger)
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

	if _, ok := cmd["stop"]; ok {
		g.logger.Info("WE ARE INSIDE THE STOP CONDITIONAL AND ARE ABOUT TO RETURN")
		return nil, g.c.Arm.Stop(ctx, nil)
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

	options := PouringOptions{}
	options.DoPour, _ = cmd["do-pour"].(bool)
	options.PickupFromFar, _ = cmd["far"].(bool)
	options.PickupFromMid, _ = cmd["mid"].(bool)

	err := g.StartPouringProcess(ctx, options)
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
	err := g.GoToPrepForPour(ctx)
	if err != nil {
		return err
	}

	err = g.c.Arm.MoveToJointPositions(ctx, JointPositionsHome, nil)
	if err != nil {
		return err
	}

	return g.c.Gripper.Open(ctx, nil)
}

func (g *Gen) GoToPrepForPour(ctx context.Context) error {
	err := g.c.Arm.MoveToJointPositions(ctx, JointPositionsPreppingForPour, nil)
	if err != nil {
		return err
	}
	return nil
}
