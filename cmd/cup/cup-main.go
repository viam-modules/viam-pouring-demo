package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/services/vision"

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

	ms := vmodutils.AddMachineFlags()

	flag.Parse()

	client, err := ms.Connect(ctx, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	cam, err := camera.FromRobot(client, "cam1")
	if err != nil {
		return err
	}

	camVision, err := vision.FromRobot(client, "circle-service")
	if err != nil {
		return err
	}

	g := pour.NewCamTesting(cam, camVision, logger)
	fmt.Printf("g %v\n", g)

	//cups, err := g.GetCupPositions(ctx)
	cups, err := g.FindCupsEliot(ctx)
	if err != nil {
		return err
	}

	for _, c := range cups {
		fmt.Printf("cup: %v\n", c)
	}

	return nil
}
