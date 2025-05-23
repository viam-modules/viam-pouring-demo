package pour

import (
	"context"
	"sort"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/spatialmath"
)

var (
	cameraToTable = 715 - plywoodHeight        // this second value is for the plywood cover that sits on top of the table
	pourOffset    = r3.Vector{40, 0, 205 - 50} // this variable controls where to pour from relative to the position of the cup as discovered by the camera
)

type PouringOptions struct {
	PickupFromFar, PickupFromMid bool // only one of these can be true, if both are false assumes on scale
	DoPour                       bool // if false just plans
}

func (g *Gen) StartPouringProcess(ctx context.Context, options PouringOptions) error {

	cupLocations, err := g.FindCupsEliot(ctx)
	if err != nil {
		return err
	}

	pourPositions := g.CameraToPourPositions(ctx, cupLocations)

	// order the cups so that we got the farthest one first and the closest one last
	pourPositions = sortByDistance(pourPositions)

	g.setStatus("found the positions of the cups, will do planning now")

	//return g.demoPlanMovements(ctx, wineBottleMeasurePoint, pourPositions, doPour)
	return g.demoPlanMovements(ctx, pourPositions, options)
}

// cupLocations is in the frame of the arm
//
//	should be the center of the rim
//
// return is in frame of arm
func (g *Gen) CameraToPourPositions(ctx context.Context, cupLocations []spatialmath.Pose) []r3.Vector {
	pourPoints := []r3.Vector{}

	for i, c := range cupLocations {
		pourLocationInArm := c.Point().Add(pourOffset)

		pourPoints = append(pourPoints, pourLocationInArm)

		g.logger.Infof("cup %d\n - cup center: %v\n - pour center in arm: %v",
			i, c, pourLocationInArm)
	}
	// panic("stop2")

	return pourPoints
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
