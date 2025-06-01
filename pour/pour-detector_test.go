package pour

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"testing"

	"go.viam.com/rdk/logging"
	"go.viam.com/test"
)

func readImage(fn string) (image.Image, error) {
	file, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func TestDetectPour1(t *testing.T) {
	logger := logging.NewTestLogger(t)

	start, err := readImage("data/pour1/img-0.png")
	test.That(t, err, test.ShouldBeNil)

	pd := newPourDetector(start)

	test.That(t, pd.delta(start), test.ShouldEqual, 0)

	img1, err := readImage("data/pour1/img-1.png")
	test.That(t, err, test.ShouldBeNil)
	d1 := pd.delta(img1)
	test.That(t, d1, test.ShouldBeGreaterThan, 0)

	img2, err := readImage("data/pour1/img-2.png")
	test.That(t, err, test.ShouldBeNil)
	d2 := pd.delta(img2)
	test.That(t, d2, test.ShouldBeGreaterThan, d1)

	img7, err := readImage("data/pour1/img-7.png")
	test.That(t, err, test.ShouldBeNil)
	d7 := pd.delta(img7)
	test.That(t, d7, test.ShouldBeGreaterThan, d2)

	for i := 0; i < 10; i++ {
		fn := fmt.Sprintf("data/pour1/img-%d.png", i)
		ii, err := readImage(fn)
		test.That(t, err, test.ShouldBeNil)
		delta := pd.delta(ii)

		logger.Infof("%v -> %v", fn, delta)

		if i < 7 {
			test.That(t, pd.different(ii), test.ShouldBeFalse)
		} else {
			test.That(t, pd.different(ii), test.ShouldBeTrue)
		}
	}

}
