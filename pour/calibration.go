package pour

import (
	"context"
	"errors"
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

func (g *gen) calibrate() error {
	ctx := context.Background()
	logger := g.logger

	// Get the camera from the robot
	realsense := g.c

	// here I need to figure out how many cups there are on the table before I proceed to figure out how many cups to look for and their positions
	b := true
	var numOfCupsToDetect int
	g.logger.Info("detecting how many cups there are on the table now")
	for b {
		num, err := g.determineAmountOfCups(context.Background())
		if errors.Is(err, errors.New("we detected a different amount of circles")) && err != nil {
			logger.Error(err)
			return err
		}
		if err == nil {
			numOfCupsToDetect = num
			b = false
		}
	}
	g.logger.Infof("WE FOUND THIS MANY CUPS: %d", numOfCupsToDetect)
	g.logger.Info("determining the positions of the cups now")
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
		g.logger.Infof("cupLocations[%d]: %v\n", i, spatialmath.PoseToProtobuf(cupLocations[i]))
	}
	g.logger.Info(" ")
	g.logger.Info(" ")

	cupDemoPoints := []r3.Vector{}
	for i := 0; i < numOfCupsToDetect; i++ {
		cupDemoPoints = append(cupDemoPoints, r3.Vector{X: cupLocations[i].Point().X, Y: cupLocations[i].Point().Y, Z: 180})
	}

	g.logger.Info("LOCATIONS IN THE FRAME OF THE ARM WITH PROPER HEIGHT")
	for i := 0; i < numOfCupsToDetect; i++ {
		g.logger.Infof("cupDemoPoints[%d]: %v\n", i, cupDemoPoints[i])
	}
	g.logger.Info(" ")
	g.logger.Info(" ")

	// HARDCODE FOR NOW
	wineBottlePoint := r3.Vector{X: -255, Y: 334, Z: 108}

	// execute the demo
	return g.demoPlanMovements(wineBottlePoint, cupDemoPoints)
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
	x := []float64{}
	y := []float64{}
	for successes := 0; successes < 20; {
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
				logger.Infof("circles[0].center: %v", circles[i].center)
				x = append(x, float64(circles[i].center.X))
				y = append(y, float64(circles[i].center.Y))
				logger.Infof(" ")
				xAdj, yAdj := determineAdjustment(logger, float64(circles[i].center.X), float64(circles[i].center.Y))
				logger.Infof("xAdj %f", xAdj)
				logger.Infof("yAdj %f", yAdj)
				pt := circleToPt(*properties.IntrinsicParams, circles[i], 715, xAdj, yAdj)
				clusters[i].include(pt)
			}
		} else {
			for _, circle := range circles {
				logger.Infof("circle.center: %v", circle.center)
				logger.Infof(" ")
				x = append(x, float64(circle.center.X))
				y = append(y, float64(circle.center.Y))
				xAdj, yAdj := determineAdjustment(logger, float64(circle.center.X), float64(circle.center.Y))
				logger.Infof("xAdj %f", xAdj)
				logger.Infof("yAdj %f", yAdj)
				pt := circleToPt(*properties.IntrinsicParams, circle, 715, xAdj, yAdj)

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

	xAvg := calculateAverage(x)
	yAvg := calculateAverage(y)
	logger.Infof("xAvg: %f", xAvg)
	logger.Infof("yAvg: %f", yAvg)

	return clusters
}

func (g *gen) determineAmountOfCups(ctx context.Context) (int, error) {
	l := make([]int, 5)
	for i := 0; i < 5; i++ {
		g.logger.Infof("on iteration %d", i)
		circ, err := getCalibrationDataPoint(ctx, g.c)
		if err != nil {
			return -1, err
		}
		g.logger.Infof("len(circ): %d", len(circ))
		l[i] = len(circ)
	}

	check := l[0]
	for _, num := range l {
		if num != check {
			return -1, errors.New("we detected a different amount of circles")
		}
	}
	return check, nil
}

func calculateAverage(numbers []float64) float64 {
	if len(numbers) == 0 {
		return 0 // Return 0 if the slice is empty to avoid division by zero
	}

	var sum float64
	for _, num := range numbers {
		sum += num
	}

	return sum / float64(len(numbers))
}

func determineAdjustment(logger logging.Logger, inputX, inputY float64) (float64, float64) {
	deltaXPos := 0.375
	deltaXNeg := 0.07
	deltaYPos := 0.2
	deltaYNeg := 0.15
	deltaX := 340.05 - inputX
	deltaY := 222.7 - inputY
	if deltaX > 0 && deltaY > 0 {
		logger.Info("deltaX > 0 && deltaY > 0")
		return deltaX * deltaXPos, deltaY * deltaYPos
	} else if deltaX > 0 && deltaY < 0 {
		logger.Info("deltaX > 0 && deltaY < 0")
		return deltaX * deltaXPos, deltaY * deltaYNeg
	} else if deltaX < 0 && deltaY < 0 {
		logger.Info("deltaX < 0 && deltaY < 0")
		return deltaX * deltaXNeg, deltaY * deltaYNeg
	}
	logger.Info("NONE OF THE CONDITINALS HIT, IN ELSE")
	return deltaX * deltaXNeg, deltaY * deltaYPos
}
