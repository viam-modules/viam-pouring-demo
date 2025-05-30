package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	vizClient "github.com/viam-labs/motion-tools/client/client"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/robot"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func main() {
	err := realMain()
	if err != nil {
		panic(err)
	}
}

func realMain() error {
	ctx := context.Background()
	logger := logging.NewLogger("cup")

	debug := false

	flag.BoolVar(&debug, "debug", false, "")
	host := flag.String("host", "", "host to connect to")
	configFile := flag.String("config", "", "host to connect to")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	if *configFile == "" {
		return fmt.Errorf("need a config file")
	}

	cfg := &pour.Config{}
	err := vmodutils.ReadJSONFromFile(*configFile, cfg)
	if err != nil {
		return err
	}

	_, _, err = cfg.Validate("")
	if err != nil {
		return err
	}

	client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	deps, err := vmodutils.MachineToDependencies(client)
	if err != nil {
		return err
	}

	p1c, err := pour.Pour1ComponentsFromDependencies(cfg, deps)
	if err != nil {
		return err
	}

	g := pour.NewTesting(logger, client, p1c)

	vc := pour.NewVinoCart(cfg, p1c, client, logger)

	cmd := flag.Arg(0)
	switch cmd {
	case "reset":
		return g.ResetArmToHome(ctx)
	case "intermediate":
		return g.GoToPrepForPour(ctx)
	case "touch":
		return vc.Touch(ctx)
	case "pour-prep":
		return vc.PourPrep(ctx)
	case "pour":
		return vc.Pour(ctx)
	case "put-back":
		return vc.PutBack(ctx)
	case "full-demo":
		err := vc.Touch(ctx)
		if err != nil {
			return err
		}
		err = vc.PourPrep(ctx)
		if err != nil {
			return err
		}
		err = vc.Pour(ctx)
		if err != nil {
			return err
		}
		return vc.PutBack(ctx)
	case "visWorldState":
		return visObstacles(ctx, client)
	case "plan":
		return g.StartPouringProcess(ctx, pour.PouringOptions{})
	case "pour-old":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true})
	case "pour-old-far":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromFar: true})
	case "pour-old-mid":
		return g.StartPouringProcess(ctx, pour.PouringOptions{DoPour: true, PickupFromMid: true})
	case "find-cups-old":
		cups, err := g.FindCups(ctx)
		if err != nil {
			return err
		}
		for idx, c := range cups {
			logger.Infof("cup %d : %v", idx, c)
		}
		return nil
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}

func visObstacles(ctx context.Context, myRobot robot.Robot) error {

	err := vizClient.RemoveAllSpatialObjects()
	if err != nil {
		return err
	}

	err = vizClient.DrawRobot(ctx, myRobot, nil)
	if err != nil {
		return err
	}

	for _, g := range pour.GenerateObstacles() {
		for _, actualGeom := range g.Geometries() {
			err = vizClient.DrawGeometry(actualGeom, "red")
			if err != nil {
				return err
			}
		}
	}

	return nil
}
