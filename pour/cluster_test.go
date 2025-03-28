package pour

import (
	"testing"

	"go.viam.com/test"
)

func TestCluster1(t *testing.T) {
	c := newCluster()
	c.include(r3.Vector{X: 1})

	v := c.mean()
	test.That(t, v.X, test.ShouldEqual, 1)
}
