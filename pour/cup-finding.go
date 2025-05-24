package pour

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
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

// returns cups in the world frame
func (g *Gen) FindCups(ctx context.Context) ([]spatialmath.Pose, error) {
	eliot, err := g.FindCupsEliot(ctx)
	if err != nil {
		return nil, err
	}

	eliot, err = g.cameraToWorldPoses(ctx, eliot)
	if err != nil {
		return nil, err
	}

	miko, err := g.FindCupsMiko(ctx)
	if err != nil {
		return nil, err
	}

	miko, err = g.cameraToWorldPoses(ctx, miko)
	if err != nil {
		return nil, err
	}

	g.logger.Infof("eliot: %v", eliot)
	g.logger.Infof("miko: %v", miko)

	return eliot, nil
}

func (g *Gen) cameraToWorldPoses(ctx context.Context, cam []spatialmath.Pose) ([]spatialmath.Pose, error) {
	tf, err := g.c.Motion.GetPose(ctx, g.c.Cam.Name(), referenceframe.World, nil, nil)
	if err != nil {
		return nil, err
	}

	world := []spatialmath.Pose{}
	for _, p := range cam {
		world = append(world, spatialmath.Compose(tf.Pose(), p))
	}

	return world, nil
}

func (g *Gen) FindCupsEliot(ctx context.Context) ([]spatialmath.Pose, error) {
	properties, err := g.c.Cam.Properties(ctx)
	if err != nil {
		return nil, err
	}

	dets, err := g.c.CamVision.DetectionsFromCamera(ctx, "", nil)
	if err != nil {
		return nil, err
	}

	imgs, _, err := g.c.Cam.Images(ctx)
	if err != nil {
		return nil, err
	}
	if len(imgs) != 2 {
		return nil, fmt.Errorf("expecting 2 images, got %d", len(imgs))
	}
	if imgs[1].SourceName != "depth" {
		return nil, fmt.Errorf("img 1 name was %s, not depth", imgs[1].SourceName)
	}

	res := []spatialmath.Pose{}
	for _, d := range dets {
		x := float64((d.BoundingBox().Min.X + d.BoundingBox().Max.X) / 2)
		y := float64((d.BoundingBox().Min.Y + d.BoundingBox().Max.Y) / 2)

		min, _ := findMinMaxIndepth(imgs[1].Image, d.BoundingBox())

		x, y, z := properties.IntrinsicParams.PixelToPoint(x, y, float64(min))

		p := spatialmath.NewPoseFromPoint(r3.Vector{X: x, Y: y, Z: z})

		g.logger.Infof("detection: %v,%v min: %v", x, y, min)

		res = append(res, p)
	}

	return res, nil
}

func findMinMaxIndepth(img image.Image, b *image.Rectangle) (int, int) {
	min := 100000
	max := 0

	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			z := int((img.At(x, y).(color.Gray16)).Y)
			if z == 0 {
				continue
			}
			if z < min {
				min = z
			}
			if z > max {
				max = z
			}
		}
	}
	return min, max
}

func (g *Gen) FindCupsMiko(ctx context.Context) ([]spatialmath.Pose, error) {
	logger := g.logger

	// here I need to figure out how many cups there are on the table before I proceed to figure out how many cups to look for and their positions
	dets, err := g.c.CamVision.DetectionsFromCamera(ctx, g.c.Cam.Name().Name, nil)
	if err != nil {
		return nil, err
	}
	numOfCupsToDetect := len(dets)
	g.setStatus("found this many cups: " + strconv.Itoa(numOfCupsToDetect) + " will now determine their postions")
	if numOfCupsToDetect == 0 {
		return nil, errors.New("there were no cups placed on the table")
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

	return cupLocations, nil
}

func (g *Gen) getTheDetections(ctx context.Context, logger logging.Logger, amountOfClusters int) []*cluster {
	properties, err := g.c.Cam.Properties(ctx)
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
		detections, err := g.c.CamVision.DetectionsFromCamera(ctx, g.c.Cam.Name().Name, nil)
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
		return deltaX * g.conf.DeltaXPos, deltaY * g.conf.DeltaYPos
	} else if deltaX > 0 && deltaY < 0 {
		logger.Info("deltaX > 0 && deltaY < 0")
		logger.Info("using deltaXPos and deltaYNeg")
		return deltaX * .22, deltaY * g.conf.DeltaYNeg
	} else if deltaX < 0 && deltaY < 0 {
		logger.Info("deltaX < 0 && deltaY < 0")
		logger.Info("using deltaXNeg and deltaYNeg")
		return deltaX * g.conf.DeltaXNeg, deltaY * g.conf.DeltaYNeg
	} else if deltaX == 0 && deltaY < 0 {
		logger.Info("deltaX == 0 && deltaY < 0")
		logger.Info("using 0 and deltaYNeg")
		return deltaX * g.conf.DeltaXNeg, deltaY * g.conf.DeltaYNeg
	} else if deltaY == 0 && deltaX < 0 {
		logger.Info("deltaY == 0 && deltaX < 0")
		logger.Info("using deltaXNeg and 0")
		return deltaX * g.conf.DeltaXNeg, deltaY * g.conf.DeltaYNeg
	} else if deltaX == 0 && deltaY > 0 {
		logger.Info("deltaX == 0 && deltaY > 0")
		logger.Info("using 0 and deltaYPos")
		return 1, deltaY * g.conf.DeltaYPos
	} else if deltaY == 0 && deltaX > 0 {
		logger.Info("deltaY == 0 && deltaX > 0")
		logger.Info("using deltaXPos and 0")
		return deltaX * g.conf.DeltaXPos, deltaY * g.conf.DeltaYNeg
	}
	logger.Info("NONE OF THE CONDITINALS HIT, IN ELSE")
	logger.Info("deltaX < 0 && deltaY > 0")
	logger.Info("using deltaXNeg and deltaYPos")
	return deltaX * 0, deltaY * g.conf.DeltaYPos
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
