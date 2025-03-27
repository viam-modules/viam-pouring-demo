package pour

import (
	"context"
	"errors"
	"image"
	"math"
	"sort"
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

func (g *gen) calibrate(ctx context.Context) error {
	// Get the camera from the robot
	realsense := g.c

	// here I need to figure out how many cups there are on the table before I proceed to figure out how many cups to look for and their positions
	dets, err := g.v.DetectionsFromCamera(ctx, realsense.Name().Name, nil)
	if err != nil {
		g.setStatus(err.Error())
		return err
	}
	numOfCupsToDetect := len(dets)
	g.setStatus("found this many cups: " + strconv.Itoa(numOfCupsToDetect) + " will now determine their postions")
	if numOfCupsToDetect == 0 {
		statement := "there were no cups placed on the table"
		g.setStatus(statement)
		return errors.New(statement)
	}

	g.logger.Infof("WE FOUND THIS MANY CUPS: %d", numOfCupsToDetect)
	g.logger.Info("determining the positions of the cups now")
	clusters := g.getTheDetections(ctx, g.logger, numOfCupsToDetect)

	// figure out which of the detections are the cups and which is the wine bottle
	// know that wrt the camera, the bottle is on the left side, so it'll have a negative X value
	cupLocations := []spatialmath.Pose{}
	for _, c := range clusters {
		cupLocations = append(cupLocations, spatialmath.NewPoseFromPoint(c.mean().Add(r3.Vector{X: 20, Y: 0, Z: 0})))
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
		cupDemoPoints = append(cupDemoPoints, r3.Vector{X: cupLocations[i].Point().X, Y: cupLocations[i].Point().Y, Z: 190})
	}

	g.logger.Info("LOCATIONS IN THE FRAME OF THE ARM WITH PROPER HEIGHT")
	for i := 0; i < numOfCupsToDetect; i++ {
		g.logger.Infof("cupDemoPoints[%d]: %v\n", i, cupDemoPoints[i])
	}
	g.logger.Info(" ")
	g.logger.Info(" ")

	// order the cups so that we got the farthest one first and the closest one last
	orderedCups := sortByDistance(cupDemoPoints)

	// HARDCODE FOR NOW
	wineBottlePoint := r3.Vector{X: -255, Y: 334, Z: 108}

	g.setStatus("found the positions of the cups, will do planning now")

	// execute the demo
	return g.demoPlanMovements(ctx, wineBottlePoint, orderedCups)
}

func (g *gen) getTheDetections(ctx context.Context, logger logging.Logger, amountOfClusters int) []*cluster {
	properties, err := g.c.Properties(ctx)
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
		detections, err := g.v.DetectionsFromCamera(ctx, g.c.Name().Name, nil)
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

func (g *gen) determineAdjustment(logger logging.Logger, inputX, inputY float64) (float64, float64) {
	deltaXPos := g.deltaXPos
	deltaXNeg := g.deltaXNeg
	deltaYPos := g.deltaYPos
	deltaYNeg := g.deltaYNeg
	logger.Infof("deltaXPos: %f", deltaXPos)
	logger.Infof("deltaYPos: %f", deltaYPos)
	logger.Infof("deltaXNeg: %f", deltaXNeg)
	logger.Infof("deltaYNeg: %f", deltaYNeg)

	deltaX := 320.1 - inputX
	deltaY := 235.1 - inputY
	logger.Infof("deltaX: %f", deltaX)
	logger.Infof("deltaY: %f", deltaY)
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
	logger.Info("deltaX < 0 && deltaY > 0")
	return deltaX * deltaXNeg, deltaY * deltaYPos
}

func circleToPt(intrinsics transform.PinholeCameraIntrinsics, circle Circle, z, xAdjustment, yAdjustment float64) r3.Vector {
	xmm := (float64(circle.center.X) - intrinsics.Ppx) * (z / intrinsics.Fx)
	ymm := (float64(circle.center.Y) - intrinsics.Ppy) * (z / intrinsics.Fy)
	xmm = xmm + xAdjustment
	ymm = ymm + yAdjustment
	return r3.Vector{X: xmm, Y: ymm, Z: z}
}

// Function to calculate the squared distance from the origin
func squaredDistance(v r3.Vector) float64 {
	return v.X*v.X + v.Y*v.Y + v.Z*v.Z
}

// Function to sort a list of r3 vectors based on distance from the origin
func sortByDistance(vectors []r3.Vector) []r3.Vector {
	// Create a custom type to hold both vector and its squared distance
	type distanceVector struct {
		vec  r3.Vector
		dist float64
	}

	// Create a slice of distanceVector
	distVecs := make([]distanceVector, len(vectors))
	for i, v := range vectors {
		distVecs[i] = distanceVector{vec: v, dist: squaredDistance(v)}
	}

	// Sort the distanceVecs slice based on the distance (in descending order)
	sort.Slice(distVecs, func(i, j int) bool {
		return distVecs[i].dist > distVecs[j].dist
	})

	// Extract the sorted vectors
	sortedVectors := make([]r3.Vector, len(vectors))
	for i, dv := range distVecs {
		sortedVectors[i] = dv.vec
	}

	return sortedVectors
}
