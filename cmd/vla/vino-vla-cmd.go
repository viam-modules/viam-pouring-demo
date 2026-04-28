package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/services/generic"
)

func main() {
	err := realMain()
	if err != nil {
		panic(err)
	}
}

func realMain() error {
	ctx := context.Background()
	logger := logging.NewLogger("vla")

	debug := false
	flag.BoolVar(&debug, "debug", false, "")
	host := flag.String("host", "", "host to connect to")
	cartName := flag.String("cart", "cart", "name of the vinocart generic service")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	vc, err := generic.FromRobot(client, *cartName)
	if err != nil {
		return err
	}

	cmd := flag.Arg(0)
	switch cmd {
	case "capture":
		capture, err := sensor.FromRobot(client, "touch-session-capture")
		if err != nil {
			return err
		}

		if _, err := capture.DoCommand(ctx, map[string]any{"start": true}); err != nil {
			return err
		}

		_, touchErr := vc.DoCommand(ctx, map[string]any{"touch": true})

		if _, err := capture.DoCommand(ctx, map[string]any{"stop": true}); err != nil {
			if touchErr != nil {
				return fmt.Errorf("touch failed: %v; also failed to stop capture: %w", touchErr, err)
			}
			return err
		}

		if touchErr != nil {
			return touchErr
		}

		_, err = vc.DoCommand(ctx, map[string]any{"reset": true})
		return err
	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}
