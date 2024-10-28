package pour

import (
	"fmt"
	"math"

	"github.com/golang/geo/r3"
)

type cluster struct {
	poses []r3.Vector
	sum   r3.Vector
}

func newCluster() *cluster {
	return &cluster{
		poses: make([]r3.Vector, 0),
		sum:   r3.Vector{},
	}
}

func (c *cluster) include(v r3.Vector) {
	c.poses = append(c.poses, v)
	c.sum = c.sum.Add(v)
}

func (c *cluster) mean() r3.Vector {
	return c.sum.Mul(1 / float64(len(c.poses)))
}

func (c *cluster) stdDev() r3.Vector {
	mean := c.mean()
	sum := r3.Vector{}
	for _, pose := range c.poses {
		diff := pose.Sub(mean)
		sum = sum.Add(r3.Vector{X: diff.X * diff.X, Y: diff.Y * diff.Y, Z: diff.Z * diff.Z})
	}
	variance := sum.Mul(1 / float64(len(c.poses)))
	return r3.Vector{X: math.Sqrt(variance.X), Y: math.Sqrt(variance.Y), Z: math.Sqrt(variance.Z)}
}

func (c *cluster) String() string {
	printR3Vec := func(v r3.Vector) string {
		return fmt.Sprintf("\t%.2f\t%.2f\t%.2f\n", v.X, v.Y, v.Z)
	}
	s := "distance(mm)\tX\tY\tZ\n"
	// for _, pose := range c.poses {
	// 	s += "\t" + printR3Vec(pose)
	// }
	s += fmt.Sprintf("average:%s", printR3Vec(c.mean()))
	return s + fmt.Sprintf("std. dev.:%s", printR3Vec(c.stdDev()))
}
