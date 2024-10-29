package pour

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/rimage"
	"go.viam.com/rdk/spatialmath"
)

func calculateThePoseTheArmShouldGoTo(transformBy, clusterPose spatialmath.Pose) spatialmath.Pose {
	return spatialmath.Compose(transformBy, clusterPose)
}

func (g *gen) calibrate() {
	ctx := context.Background()
	logger := logging.NewLogger("client")

	// Get the camera from the robot
	realsense := g.c

	// get the data
	numOfCupsToDetect := 1
	clusters := getTheDetections(ctx, realsense, logger, numOfCupsToDetect)

	// figure out which of the detections are the cups and which is the wine bottle
	// know that wrt the camera, the bottle is on the left side, so it'll have a negative X value
	cupLocations := []spatialmath.Pose{}
	for _, c := range clusters {
		cupLocations = append(cupLocations, spatialmath.NewPoseFromPoint(c.mean()))
	}
	g.logger.Info(" ")
	g.logger.Info(" ")
	g.logger.Info("LOCATIONS IN THE FRAME OF THE CAMERA")
	for i := 0; i < numOfCupsToDetect; i++ {
		g.logger.Infof("cupLocations[%d]: %v\n", i, spatialmath.PoseToProtobuf(cupLocations[i]))
	}

	motionService := g.m
	g.logger.Info(" ")
	g.logger.Info(" ")
	g.logger.Info(" ")
	g.logger.Info(" ")

	// get the transform from camera frame to the world frame
	tf, _ := motionService.GetPose(ctx, realsense.Name(), referenceframe.World, nil, nil)

	for i := 0; i < numOfCupsToDetect; i++ {
		cupLocations[i] = calculateThePoseTheArmShouldGoTo(tf.Pose(), cupLocations[i])
	}

	g.logger.Info("LOCATIONS IN THE FRAME OF THE ARM")
	for i := 0; i < numOfCupsToDetect; i++ {
		fmt.Printf("cupLocations[%d]: %v\n", i, spatialmath.PoseToProtobuf(cupLocations[i]))
	}
	g.logger.Info(" ")
	g.logger.Info(" ")

	cupDemoPoints := []r3.Vector{}
	for i := 0; i < numOfCupsToDetect; i++ {
		cupDemoPoints = append(cupDemoPoints, r3.Vector{X: cupLocations[i].Point().X, Y: cupLocations[i].Point().Y, Z: 230})
	}

	g.logger.Info("LOCATIONS IN THE FRAME OF THE ARM WITH PROPER HEIGHT")
	for i := 0; i < numOfCupsToDetect; i++ {
		fmt.Printf("cupDemoPoints[%d]: %v\n", i, cupDemoPoints[i])
	}
	g.logger.Info(" ")
	g.logger.Info(" ")

	// fmt.Print("Validate that the positions are valid, press 'Enter' to continue...")
	// bufio.NewReader(os.Stdin).ReadBytes('\n')

	// HARDCODE FOR NOW
	wineBottlePoint := r3.Vector{X: -255, Y: 334, Z: 108}
	_ = wineBottlePoint

	// execute the demo
	g.demoPlanMovements(wineBottlePoint, cupDemoPoints)
}

func getCalibrationDataPoint(ctx context.Context, cam camera.Camera) ([]Circle, error) {
	images, _, err := cam.Images(ctx)
	if err != nil {
		return nil, err
	}
	for _, img := range images {
		if img.SourceName == "depth" {
			return vesselCircles(img.Image)
		} else {
			// this is what the camera saw in RGBA

			crop := image.Rectangle{Min: image.Pt(65, 0), Max: image.Pt(640, 320)}
			// Create a new RGBA image with the size of the crop rectangle
			croppedImg := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))

			// Adjust the draw point to correctly position the cropped area
			draw.Draw(croppedImg, croppedImg.Bounds(), img.Image, crop.Min, draw.Src)

			rimage.SaveImage(img.Image, "real_time_image.jpg")
		}
	}
	return nil, errors.New("this shouldn't happen")
}

func getTheDetections(ctx context.Context, realsense camera.Camera, logger logging.Logger, amountOfClusters int) []*cluster {
	properties, err := realsense.Properties(ctx)
	if err != nil {
		logger.Fatal(err)
	}

	clusters := make([]*cluster, amountOfClusters)
	logger.Infof("len(clusters): %d", len(clusters))
	for i := range len(clusters) {
		clusters[i] = newCluster()
	}
	for successes := 0; successes < 10; {
		logger.Infof("attempting calibration iteration: %d", successes)
		circles, err := getCalibrationDataPoint(ctx, realsense)
		if err != nil {
			logger.Fatal(err)
		}
		if len(circles) != amountOfClusters {
			continue
		}
		if successes == 0 {
			for i := range len(clusters) {
				clusters[i].include(circleToPt(*properties.IntrinsicParams, circles[0], float64(maxDepth)))
			}
		} else {
			for _, circle := range circles {
				pt := circleToPt(*properties.IntrinsicParams, circle, float64(maxDepth))
				min := math.Inf(1)
				minIdx := 0
				for i, cluster := range clusters {
					dist := cluster.mean().Distance(pt)
					if dist <= min {
						min = dist
						minIdx = i
					}
				}
				clusters[minIdx].include(pt)
			}
		}
		successes++
	}

	checkLength := len(clusters[0].poses)
	for i := range len(clusters) {
		logger.Infof("len(clusters[i].poses): %v", len(clusters[i].poses))
		logger.Infof("checkLength: %v", checkLength)
		if len(clusters[i].poses) != checkLength {
			logger.Fatal("clusters not of equal length")
		}
	}

	return clusters
}
