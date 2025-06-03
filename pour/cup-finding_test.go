package pour

import (
	"image"
	"math"
	"testing"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/rimage/transform"
	"go.viam.com/test"
)

// center - 50.3

var realsenseIntrinsics = transform.PinholeCameraIntrinsics{640, 480, 604.0442504882812, 603.4156494140625, 324.6331481933594, 249.64450073242188}

func dist(x, y float64) float64 {
	return math.Pow((x*x)+(y*y), .5)
}

func TestCam3da(t *testing.T) {
	t.Skip() // TODO ??
	logger := logging.NewTestLogger(t)

	x, y, z := realsenseIntrinsics.PixelToPoint(float64(realsenseIntrinsics.Width/2), float64(realsenseIntrinsics.Height/2), 50.3)

	logger.Infof("hi %v %v %v", x, y, z)
	test.That(t, x, test.ShouldAlmostEqual, 0, 1)
	test.That(t, y, test.ShouldAlmostEqual, 0, 1)
	test.That(t, z, test.ShouldAlmostEqual, 50.3, 1)

	x, y, z = realsenseIntrinsics.PixelToPoint(406, 127, 51.3)

	logger.Infof("hi %v %v %v - %v", x, y, z, dist(x, y))
	test.That(t, dist, test.ShouldAlmostEqual, 12.1, .5)
	test.That(t, z, test.ShouldAlmostEqual, 50.3, 1)
}

func weirdVersion(logger logging.Logger, x, y, z float64) (float64, float64, float64) {
	g := &Gen{conf: &Config{
		DeltaXNeg: 0.295,
		DeltaXPos: 0.295,
		DeltaYNeg: 0.295,
		DeltaYPos: 0.295,
	}}

	xAdjustment, yAdjustment := g.determineAdjustment(logger, x, y)
	pp := circleToPt(realsenseIntrinsics, Circle{image.Point{int(x), int(y)}, 10}, z, xAdjustment, yAdjustment)
	return pp.X, pp.Y, z
}

func TestCam3d1(t *testing.T) {
	t.Skip() // TODO ??
	logger := logging.NewTestLogger(t)

	x := 406.0
	y := 127.0
	zz := 51.3

	xa, ya, _ := realsenseIntrinsics.PixelToPoint(x, y, zz)
	xb, yb, _ := weirdVersion(logger, x, y, zz)

	logger.Infof("x  a:  %v   b: %v", xa, xb)
	logger.Infof("y  a:  %v   b: %v", ya, yb)

	test.That(t, xa, test.ShouldAlmostEqual, xb)
	test.That(t, ya, test.ShouldAlmostEqual, yb)

}

func TestCam3d2(t *testing.T) {
	t.Skip() // TODO ??
	logger := logging.NewTestLogger(t)

	x := 391.0
	y := 34.0
	zz := 53.0

	xa, ya, _ := realsenseIntrinsics.PixelToPoint(x, y, zz)
	xb, yb, _ := weirdVersion(logger, x, y, zz)

	dista := dist(xa, ya)
	distb := dist(xb, yb)

	logger.Infof("x  a:  %v   b: %v", xa, xb)
	logger.Infof("y  a:  %v   b: %v", ya, yb)
	logger.Infof("d  a:  %v   b: %v", dista, distb)

	test.That(t, xa, test.ShouldAlmostEqual, xb)
	test.That(t, ya, test.ShouldAlmostEqual, yb)

}

func TestFindSingleCupInPointCloud(t *testing.T) {
	logger := logging.NewTestLogger(t)

	in, err := pointcloud.NewFromFile("data/cupbad1.pcd", "")
	test.That(t, err, test.ShouldBeNil)

	expectedRadius := 85.0
	expectedHeight := 121.0

	in, err = cleanPointCloud(in)
	test.That(t, err, test.ShouldBeNil)

	center, height, radius, ok := findSingleCupInCleanedPointCloud(in, expectedRadius, expectedHeight, 20, logger)
	test.That(t, ok, test.ShouldBeTrue)
	test.That(t, height, test.ShouldAlmostEqual, expectedHeight, 15)
	test.That(t, radius, test.ShouldAlmostEqual, expectedRadius, 20)

	test.That(t, center.Z, test.ShouldAlmostEqual, 35+(120-35)/2, 10)

	test.That(t, center.X, test.ShouldAlmostEqual, -413.47, 5)
	test.That(t, center.Y, test.ShouldAlmostEqual, -240.7, 20) // this is probably too far

}

func TestFindSingleCupInPointCloud2(t *testing.T) {

	for _, fn := range []string{"data/cupbad2.pcd"} {
		t.Run(fn, func(t *testing.T) {
			logger := logging.NewTestLogger(t)

			in, err := pointcloud.NewFromFile(fn, "")
			test.That(t, err, test.ShouldBeNil)

			expectedRadius := 35.0
			expectedHeight := 120.0

			in, err = cleanPointCloud(in)
			test.That(t, err, test.ShouldBeNil)

			center, height, radius, ok := findSingleCupInCleanedPointCloud(in, expectedRadius, expectedHeight, 20, logger)
			test.That(t, ok, test.ShouldBeTrue)
			test.That(t, height, test.ShouldAlmostEqual, expectedHeight, 15)
			test.That(t, radius, test.ShouldAlmostEqual, expectedRadius, 20)

			test.That(t, center.Z, test.ShouldAlmostEqual, 35+(120-35)/2, 10)
		})
	}
}
