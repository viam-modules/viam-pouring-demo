package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/erh/vmodutils"

	"go.viam.com/rdk/app"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
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
	host := flag.String("host", "", "host to connect to (also drives sessions-<host>.csv path)")

	since := flag.Duration("since", 24*time.Hour, "export: only include sessions started within this window (0 = all)")
	outDir := flag.String("out", "openvla-export", "output directory (export and capture-direct)")
	armName := flag.String("arm", "left-arm", "arm component name (export only, for JointPositions data)")
	gripperName := flag.String("gripper", "left-gripper", "gripper component name (capture-direct: frame system → world pose)")
	camName := flag.String("cam", "right-cam", "primary camera component name")
	cam2Name := flag.String("cam2", "", "second camera component name (optional)")
	cam3Name := flag.String("cam3", "", "third camera component name (optional)")
	instruction := flag.String("instruction", "grab the cup", "language instruction")
	hz := flag.Int("hz", 10, "capture-direct: sample rate (Hz) for arm and camera")

	flag.Parse()

	if debug {
		logger.SetLevel(logging.DEBUG)
	}

	if *host == "" {
		return fmt.Errorf("-host is required")
	}
	sessionsPath := sessionsPathFor(*host)

	cmd := flag.Arg(0)

	switch cmd {
	case "capture":
		client, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
		if err != nil {
			return err
		}
		defer client.Close(ctx)

		// vc, err := generic.FromRobot(client, *cartName)
		// if err != nil {
		// 	return err
		// }

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

		startTS := time.Now()
		// _, touchErr := vc.DoCommand(ctx, map[string]any{"touch": true})
		endTS := time.Now()

		// if _, err := capture.DoCommand(ctx, map[string]any{"stop": true}); err != nil {
		// 	if touchErr != nil {
		// 		return fmt.Errorf("touch failed: %v; also failed to stop capture: %w", touchErr, err)
		// 	}
		// 	return err
		// }

		// if touchErr != nil {
		// 	return touchErr
		// }

		if err := appendSession(sessionsPath, startTS, endTS); err != nil {
			return fmt.Errorf("recording session: %w", err)
		}
		logger.Infof("recorded session [%s, %s] to %s", startTS.Format(time.RFC3339Nano), endTS.Format(time.RFC3339Nano), sessionsPath)

		// _, err = vc.DoCommand(ctx, map[string]any{"reset": true})
		return err

	case "capture-direct":
		connectCtx, connectCancel := context.WithTimeout(ctx, 15*time.Second)
		defer connectCancel()
		logger.Infof("connecting to %s...", *host)
		client, err := vmodutils.ConnectToHostFromCLIToken(connectCtx, *host, logger)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %w", *host, err)
		}
		defer client.Close(ctx)

		camNames := []string{*camName}
		if *cam2Name != "" {
			camNames = append(camNames, *cam2Name)
		}
		if *cam3Name != "" {
			camNames = append(camNames, *cam3Name)
		}

		return runCaptureDirect(ctx, client, captureDirectOptions{
			outDir:      *outDir,
			gripperName: *gripperName,
			camNames:    camNames,
			instruction: *instruction,
			hz:          *hz,
		}, logger)

	case "export":
		machine, err := vmodutils.ConnectToHostFromCLIToken(ctx, *host, logger)
		if err != nil {
			return err
		}
		md, err := machine.CloudMetadata(ctx)
		if err != nil {
			machine.Close(ctx)
			return fmt.Errorf("CloudMetadata: %w", err)
		}
		machine.Close(ctx)
		logger.Infof("cloud metadata for %s: location=%s part=%s", *host, md.LocationID, md.MachinePartID)

		appClient, err := app.ConnectFromCLIToken(ctx, logger)
		if err != nil {
			return fmt.Errorf("export needs viam cli credentials (run `viam login` first): %w", err)
		}
		defer appClient.Close()

		return runExport(ctx, appClient.DataClient(), exportOptions{
			sessionsPath: sessionsPath,
			since:        *since,
			outDir:       *outDir,
			armName:      *armName,
			camName:      *camName,
			instruction:  *instruction,
			locationID:   md.LocationID,
			partID:       md.MachinePartID,
		}, logger)

	default:
		return fmt.Errorf("unknown command: %v", cmd)
	}
}

func sessionsPathFor(host string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '.', r == '_':
			return r
		}
		return '_'
	}, host)
	return fmt.Sprintf("sessions-%s.csv", safe)
}

func appendSession(path string, start, end time.Time) error {
	_, statErr := os.Stat(path)
	isNew := os.IsNotExist(statErr)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if isNew {
		if err := w.Write([]string{"session_id", "start", "end"}); err != nil {
			return err
		}
	}

	id := start.UTC().Format("20060102T150405.000Z")
	return w.Write([]string{
		id,
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	})
}
