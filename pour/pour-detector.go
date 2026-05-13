package pour

import (
	"image"
	"image/color"
	"math"
)

const deltaHardCode = 1.0

// baselineLookbackFrames controls how far back the rolling-baseline detector
// looks when computing per-window delta. At a 100 ms pour loop this is a
// 500 ms window, so slow drift (camera AGC, bottle shadow shifting) cancels
// out while a sudden event (liquid hitting the cup) produces a clear spike.
const baselineLookbackFrames = 5

type pourDetector struct {
	firstImage image.Image
	hash       float64
	history    []float64
}

func newPourDetector(img image.Image) *pourDetector {
	x := computeGrayscaleAverage(img)
	return &pourDetector{
		firstImage: img,
		hash:       x,
		history:    []float64{x},
	}
}

func (pd *pourDetector) differentDebug(img image.Image) (float64, bool) {
	d := pd.deltaRolling(img)
	return d, d > deltaHardCode
}

func (pd *pourDetector) different(img image.Image) bool {
	return pd.delta(img) > deltaHardCode
}

func (pd *pourDetector) delta(img image.Image) float64 {
	hash := computeGrayscaleAverage(img)
	return math.Abs(hash - pd.hash)
}

// deltaRolling compares the current frame to a frame from roughly
// baselineLookbackFrames frames ago, instead of the first frame of the pour.
// This eliminates the drift accumulation that fixed-baseline comparison
// suffers from when the bottle/shadows/exposure change gradually during a pour.
func (pd *pourDetector) deltaRolling(img image.Image) float64 {
	hash := computeGrayscaleAverage(img)
	baseline := pd.history[0]
	pd.history = append(pd.history, hash)
	if len(pd.history) > baselineLookbackFrames+1 {
		pd.history = pd.history[1:]
	}
	return math.Abs(hash - baseline)
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
