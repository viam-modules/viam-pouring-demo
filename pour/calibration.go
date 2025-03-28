package pour

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"

	"github.com/golang/geo/r3"
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

func (g *gen) startPouringProcess(ctx context.Context, doPour bool) error {

	// here I need to figure out how many cups there are on the table before I proceed to figure out how many cups to look for and their positions
	dets, err := g.camVision.DetectionsFromCamera(ctx, "", nil)
	if err != nil {
		g.setStatus(err.Error())
		return err
	}
	numOfCupsToDetect := len(dets)
	g.setStatus("found this many cups: " + strconv.Itoa(numOfCupsToDetect) + " will now determine their postions")

	if numOfCupsToDetect == 0 {
		return fmt.Errorf("there were no cups placed on the table")
	}

	clusters, err := g.getTheDetections(ctx, numOfCupsToDetect)
	if err != nil {
		return err
	}

	// figure out which of the detections are the cups and which is the wine bottle
	// know that wrt the camera, the bottle is on the left side, so it'll have a negative X value
	cupLocations := []spatialmath.Pose{}
	for _, c := range clusters {
		cupLocations = append(cupLocations, spatialmath.NewPoseFromPoint(c.mean().Add(r3.Vector{X: 20, Y: 0, Z: 0})))
	}

	g.logger.Info("LOCATIONS IN THE FRAME OF THE CAMERA")
	for i := 0; i < numOfCupsToDetect; i++ {
		g.logger.Infof("cupLocations[%d]: %v\n", i, spatialmath.PoseToProtobuf(cupLocations[i]))
	}

	// get the transform from camera frame to the world frame
	tf, _ := g.motion.GetPose(ctx, g.cam.Name(), referenceframe.World, nil, nil)

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

	// order the cups so that we got the farthest one first and the closest one last
	orderedCups := sortByDistance(cupDemoPoints)

	// HARDCODE FOR NOW
	wineBottlePoint := r3.Vector{X: -255, Y: 334, Z: 108}

	g.setStatus("found the positions of the cups, will do planning now")

	// execute the demo
	return g.demoPlanMovements(ctx, wineBottlePoint, orderedCups, doPour)
}

func (g *gen) getTheDetections(ctx context.Context, amountOfClusters int) ([]*cluster, error) {
	properties, err := g.cam.Properties(ctx)
	if err != nil {
		return nil, err
	}

	clusters := make([]*cluster, amountOfClusters)
	g.logger.Infof("len(clusters): %d", len(clusters))
	for i := range len(clusters) {
		clusters[i] = newCluster()
	}
	x := []float64{}
	y := []float64{}
	for successes := 0; successes < 20; {
		g.logger.Infof("attempting calibration iteration: %d", successes)
		detections, err := g.camVision.DetectionsFromCamera(ctx, "", nil) // TODO-eliot
		if err != nil {
			return nil, err
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
				g.logger.Infof("circles[0].center: %v", circles[i].center)
				x = append(x, float64(circles[i].center.X))
				y = append(y, float64(circles[i].center.Y))

				xAdj, yAdj := g.determineAdjustment(float64(circles[i].center.X), float64(circles[i].center.Y))
				g.logger.Infof("xAdj %f", xAdj)
				g.logger.Infof("yAdj %f", yAdj)
				pt := circleToPt(*properties.IntrinsicParams, circles[i], 715, xAdj, yAdj)
				clusters[i].include(pt)
			}
		} else {
			for _, circle := range circles {
				g.logger.Infof("circle.center: %v", circle.center)
				x = append(x, float64(circle.center.X))
				y = append(y, float64(circle.center.Y))
				xAdj, yAdj := g.determineAdjustment(float64(circle.center.X), float64(circle.center.Y))
				g.logger.Infof("xAdj %f", xAdj)
				g.logger.Infof("yAdj %f", yAdj)
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
	g.logger.Infof("xAvg: %f", xAvg)
	g.logger.Infof("yAvg: %f", yAvg)

	return clusters, nil
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

func (g *gen) determineAdjustment(inputX, inputY float64) (float64, float64) {
	g.logger.Infof("deltaXPos: %f", g.conf.DeltaXPos)
	g.logger.Infof("deltaYPos: %f", g.conf.DeltaYPos)
	g.logger.Infof("deltaXNeg: %f", g.conf.DeltaXNeg)
	g.logger.Infof("deltaYNeg: %f", g.conf.DeltaYNeg)

	deltaX := 320.1 - inputX
	deltaY := 235.1 - inputY
	g.logger.Infof("deltaX: %f", deltaX)
	g.logger.Infof("deltaY: %f", deltaY)
	if deltaX > 0 && deltaY > 0 {
		g.logger.Info("deltaX > 0 && deltaY > 0")
		return deltaX * g.conf.DeltaXPos, deltaY * g.conf.DeltaYPos
	} else if deltaX > 0 && deltaY < 0 {
		g.logger.Info("deltaX > 0 && deltaY < 0")
		return deltaX * g.conf.DeltaXPos, deltaY * g.conf.DeltaYNeg
	} else if deltaX < 0 && deltaY < 0 {
		g.logger.Info("deltaX < 0 && deltaY < 0")
		return deltaX * g.conf.DeltaXNeg, deltaY * g.conf.DeltaYNeg
	}
	g.logger.Info("NONE OF THE CONDITINALS HIT, IN ELSE")
	g.logger.Info("deltaX < 0 && deltaY > 0")
	return deltaX * g.conf.DeltaXNeg, deltaY * g.conf.DeltaYPos
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
