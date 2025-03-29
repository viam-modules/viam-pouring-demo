package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"

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
	logger := logging.NewLogger("scale")

	ms := vmodutils.AddMachineFlags()

	flag.Parse()

	client, err := ms.Connect(ctx, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	scale, err := sensor.FromRobot(client, "scale1")
	if err != nil {
		return err
	}

	ws, err := pour.NewWeightSmoother(ctx, sensor.Named("foo"), scale, logger)
	if err != nil {
		return err
	}

	res, err := ws.Go(ctx, 10, 25, "mass_kg")
	if err != nil {
		return err
	}

	fmt.Printf("res %v\n", res)

	return nil
}
