package pour

import (
	"image"
	"image/color"
	"math"
)

const deltaHardCode = 1.0

type pourDetector struct {
	firstImage image.Image
	hash       float64
}

func newPourDetector(img image.Image) *pourDetector {
	x := computeGrayscaleAverage(img)
	return &pourDetector{img, x}
}

func (pd *pourDetector) differentDebug(img image.Image) (float64, bool) {
	d := pd.delta(img)
	return d, d > deltaHardCode
}

func (pd *pourDetector) different(img image.Image) bool {
	return pd.delta(img) > deltaHardCode
}

func (pd *pourDetector) delta(img image.Image) float64 {
	hash := computeGrayscaleAverage(img)
	return math.Abs(hash - pd.hash)
}

func computeGrayscaleAverage(img image.Image) float64 {
	bounds := img.Bounds()

	totalValue := 0.0
	numPixels := 0.0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			grayColor := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			totalValue += float64(grayColor.Y)
			numPixels++
		}
	}

	return totalValue / numPixels
}
