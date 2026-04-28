package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/app"
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

	since := flag.Duration("since", 24*time.Hour, "export: how far back to fetch data")
	outDir := flag.String("out", "openvla-export", "export: output directory")
	tagPrefix := flag.String("tag-prefix", "touch-capture-", "export: session capture tag prefix")
	armName := flag.String("arm", "left-arm", "export: arm component name (JointPositions → observation.state)")
	camName := flag.String("cam", "right-cam", "export: camera component name (GetImages → observation.image)")
	instruction := flag.String("instruction", "touch the wine bottle", "export: language instruction")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	cmd := flag.Arg(0)

	switch cmd {
	case "capture":
		client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
		if err != nil {
			return err
		}
		defer client.Close(ctx)

		vc, err := generic.FromRobot(client, *cartName)
		if err != nil {
			return err
		}

		capture, err := sensor.FromRobot(client, "touch-session-capture")
		if err != nil {
			return err
		}

		if _, err := capture.DoCommand(ctx, map[string]any{"start": true}); err != nil {
			return err
		}

		readings, err := capture.Readings(ctx, nil)
		if err != nil {
			return fmt.Errorf("readings after start: %w", err)
		}
		logger.Infof("touch-session-capture readings: %v", readings)

		time.Sleep(5 * time.Second)

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

	case "export":
		appClient, err := app.ConnectFromCLIToken(ctx, logger)
		if err != nil {
			return fmt.Errorf("export needs viam cli credentials (run `viam login` first): %w", err)
		}
		defer appClient.Close()

		return runExport(ctx, appClient.DataClient(), exportOptions{
			since:       *since,
			outDir:      *outDir,
			tagPrefix:   *tagPrefix,
			armName:     *armName,
			camName:     *camName,
			instruction: *instruction,
		}, logger)

	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}
