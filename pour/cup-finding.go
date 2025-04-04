package pour

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/rimage/transform"
	"go.viam.com/rdk/spatialmath"
)

type Circle struct {
	center image.Point
	radius int
}

func calculateThePoseTheArmShouldGoTo(transformBy, clusterPose spatialmath.Pose) spatialmath.Pose {
	return spatialmath.Compose(transformBy, clusterPose)
}

func (g *Gen) FindCupsEliot(ctx context.Context) ([]spatialmath.Pose, error) {
	logger := g.logger

	// Get the camera from the robot
	realsense := g.cam

	// here I need to figure out how many cups there are on the table before I proceed to figure out how many cups to look for and their positions
	dets, err := g.camVision.DetectionsFromCamera(ctx, realsense.Name().Name, nil)
	if err != nil {
		g.setStatus(err.Error())
		return nil, err
	}
	numOfCupsToDetect := len(dets)
	g.setStatus("found this many cups: " + strconv.Itoa(numOfCupsToDetect) + " will now determine their postions")
	if numOfCupsToDetect == 0 {
		statement := "there were no cups placed on the table"
		g.setStatus(statement)
		return nil, errors.New(statement)
	}

	g.logger.Infof("WE FOUND THIS MANY CUPS: %d", numOfCupsToDetect)
	g.logger.Info("determining the positions of the cups now")
	clusters := g.getTheDetections(ctx, logger, numOfCupsToDetect)

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

	motionService := g.motion
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
	// panic("stop")
	return cupLocations, nil
}

func (g *Gen) getTheDetections(ctx context.Context, logger logging.Logger, amountOfClusters int) []*cluster {
	properties, err := g.cam.Properties(ctx)
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
	for successes := 0; successes < 5; {
		logger.Infof("attempting calibration iteration: %d", successes)
		detections, err := g.camVision.DetectionsFromCamera(ctx, g.cam.Name().Name, nil)
		if err != nil {
			logger.Fatal(err)
		}
		circles := make([]Circle, len(detections))
		for i, d := range detections {
			xAvg := (d.BoundingBox().Min.X + d.BoundingBox().Max.X) / 2
			yAvg := (d.BoundingBox().Min.Y + d.BoundingBox().Max.Y) / 2
			circles[i] = Circle{
				center: image.Point{X: xAvg, Y: yAvg},
				radius: xAvg,
			}
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
				xAdj, yAdj := g.determineAdjustment(logger, float64(circles[i].center.X), float64(circles[i].center.Y))
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
				xAdj, yAdj := g.determineAdjustment(logger, float64(circle.center.X), float64(circle.center.Y))
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

func (g *Gen) determineAdjustment(logger logging.Logger, inputX, inputY float64) (float64, float64) {
	// deltaXNeg := 0.2
	// deltaXPos := 0.2
	// deltaYNeg := 0.295
	// deltaYPos := 0.295
	deltaXNeg := 0.2
	deltaXPos := 0.325
	deltaYNeg := 0.295
	deltaYPos := 0.325
	logger.Infof("deltaXPos: %f", deltaXPos)
	logger.Infof("deltaYPos: %f", deltaYPos)
	logger.Infof("deltaXNeg: %f", deltaXNeg)
	logger.Infof("deltaYNeg: %f", deltaYNeg)
	logger.Infof("hi there lol")

	// 313,225
	deltaX := 313 - inputX
	deltaY := 225 - inputY
	if math.Abs(deltaX) < 7 {
		deltaX = 0
	}
	logger.Infof("deltaX: %f", deltaX)
	logger.Infof("deltaY: %f", deltaY)
	if deltaX > 0 && deltaY > 0 {
		logger.Info("deltaX > 0 && deltaY > 0")
		logger.Info("using deltaXPos and deltaYPos")
		return deltaX * deltaXPos, deltaY * deltaYPos
	} else if deltaX > 0 && deltaY < 0 {
		logger.Info("deltaX > 0 && deltaY < 0")
		logger.Info("using deltaXPos and deltaYNeg")
		deltaXPos = 0.22
		return deltaX * deltaXPos, deltaY * deltaYNeg
	} else if deltaX < 0 && deltaY < 0 {
		logger.Info("deltaX < 0 && deltaY < 0")
		logger.Info("using deltaXNeg and deltaYNeg")
		return deltaX * deltaXNeg, deltaY * deltaYNeg
	} else if deltaX == 0 && deltaY < 0 {
		logger.Info("deltaX == 0 && deltaY < 0")
		logger.Info("using 0 and deltaYNeg")
		return deltaX * deltaXNeg, deltaY * deltaYNeg
	} else if deltaY == 0 && deltaX < 0 {
		logger.Info("deltaY == 0 && deltaX < 0")
		logger.Info("using deltaXNeg and 0")
		return deltaX * deltaXNeg, deltaY * deltaYNeg
	} else if deltaX == 0 && deltaY > 0 {
		logger.Info("deltaX == 0 && deltaY > 0")
		logger.Info("using 0 and deltaYPos")
		return 1, deltaY * deltaYPos
	} else if deltaY == 0 && deltaX > 0 {
		logger.Info("deltaY == 0 && deltaX > 0")
		logger.Info("using deltaXPos and 0")
		return deltaX * deltaXPos, deltaY * deltaYNeg
	}
	logger.Info("NONE OF THE CONDITINALS HIT, IN ELSE")
	logger.Info("deltaX < 0 && deltaY > 0")
	logger.Info("using deltaXNeg and deltaYPos")
	return deltaX * 0, deltaY * deltaYPos
}

func circleToPt(intrinsics transform.PinholeCameraIntrinsics, circle Circle, z, xAdjustment, yAdjustment float64) r3.Vector {
	xmm := (float64(circle.center.X) - intrinsics.Ppx) * (z / intrinsics.Fx)
	ymm := (float64(circle.center.Y) - intrinsics.Ppy) * (z / intrinsics.Fy)
	fmt.Println("xAdjustment: ", xAdjustment)
	fmt.Println("yAdjustment: ", yAdjustment)
	xmm = xmm + xAdjustment
	ymm = ymm + yAdjustment
	return r3.Vector{X: xmm, Y: ymm, Z: z}
}
