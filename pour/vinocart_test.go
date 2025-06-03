package pour

import (
	"testing"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/spatialmath"
	"go.viam.com/test"
)

func TestGetApproachPoint(t *testing.T) {

	logger := logging.NewTestLogger(t)

	md := pointcloud.MetaData{
		MinX: 200,
		MaxX: 267.7,
		MinY: 164,
		MaxY: 239.4,
	}
	c := r3.Vector{233.8, 202.05, 85.0}

	p := getApproachPoint(md, c, 100, &spatialmath.OrientationVectorDegrees{OX: 1}, logger)
	test.That(t, p.X, test.ShouldAlmostEqual, 100, 1)
	test.That(t, p.Y, test.ShouldAlmostEqual, c.Y, 1)
	test.That(t, p.Z, test.ShouldAlmostEqual, c.Z, 1)

	p = getApproachPoint(md, c, 100, &spatialmath.OrientationVectorDegrees{OX: -1}, logger)
	test.That(t, p.X, test.ShouldAlmostEqual, 367.7, 1)
	test.That(t, p.Y, test.ShouldAlmostEqual, c.Y, 1)
	test.That(t, p.Z, test.ShouldAlmostEqual, c.Z, 1)

	p = getApproachPoint(md, c, 100, &spatialmath.OrientationVectorDegrees{OY: 1}, logger)
	test.That(t, p.X, test.ShouldAlmostEqual, c.X, 1)
	test.That(t, p.Y, test.ShouldAlmostEqual, md.MinY-100, 1)
	test.That(t, p.Z, test.ShouldAlmostEqual, c.Z, 1)

	p = getApproachPoint(md, c, 100, &spatialmath.OrientationVectorDegrees{OY: -1}, logger)
	test.That(t, p.X, test.ShouldAlmostEqual, c.X, 1)
	test.That(t, p.Y, test.ShouldAlmostEqual, md.MaxY+100, 1)
	test.That(t, p.Z, test.ShouldAlmostEqual, c.Z, 1)

}
