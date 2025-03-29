package pour

import (
	"testing"

	"go.viam.com/test"
)

func TestGetBestNumberForWeight(t *testing.T) {
	test.That(t, 5, test.ShouldAlmostEqual, getBestNumberForWeight([]float64{5}))
	test.That(t, 5, test.ShouldAlmostEqual, getBestNumberForWeight([]float64{5, 5, 5}))
	test.That(t, 5, test.ShouldAlmostEqual, getBestNumberForWeight([]float64{4, 5, 6}))
	test.That(t, 5, test.ShouldAlmostEqual, getBestNumberForWeight([]float64{0, 0, 5, 5, 5, 5, 5, 10, 10}))

}
