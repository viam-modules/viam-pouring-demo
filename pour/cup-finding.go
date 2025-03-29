package pour

import (
	"context"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/spatialmath"
)

func (g *Gen) FindCupsEliot(ctx context.Context) ([]spatialmath.Pose, error) {
	properties, err := g.cam.Properties(ctx)
	if err != nil {
		return nil, err
	}

	dets, err := g.camVision.DetectionsFromCamera(ctx, "", nil)
	if err != nil {
		return nil, err
	}

	res := []spatialmath.Pose{}
	for _, d := range dets {
		x := float64((d.BoundingBox().Min.X + d.BoundingBox().Max.X) / 2)
		y := float64((d.BoundingBox().Min.Y + d.BoundingBox().Max.Y) / 2)

		x, y, z := properties.IntrinsicParams.PixelToPoint(x, y, cameraToTable-g.conf.CupHeight) // eww
		res = append(res, spatialmath.NewPoseFromPoint(r3.Vector{X: x, Y: y, Z: z}))
	}

	return res, nil
}
