package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/components/arm"
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
	logger := logging.NewLogger("cup")

	ms := vmodutils.AddMachineFlags()

	flag.Parse()

	client, err := ms.Connect(ctx, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	arm, err := arm.FromRobot(client, "arm")
	if err != nil {
		return err
	}

	cmd := flag.Arg(0)
	switch cmd {
	case "reset":
		return pour.ResetArmToHome(ctx, logger, arm)
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}

	return nil
}
