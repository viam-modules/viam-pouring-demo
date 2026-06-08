package pour

import (
	"fmt"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/pointcloud"
)

// CupGeometry is the measured geometry of a cup, derived directly from a point
// cloud whose every point is assumed to belong to the cup. The cloud is
// expected in the world frame with the table at Z=0 (same as the rest of the
// demo), so RimZ is the cup rim's absolute height in world coordinates.
type CupGeometry struct {
	Center    r3.Vector // bounding-box center; X,Y give the grasp axis
	RimZ      float64   // world Z of the rim (cloud MaxZ), used for approach height
	Height    float64   // physical height (MaxZ-MinZ), used for validation
	Width     float64   // mean of the X and Y extents
	NumPoints int
}

// cupGeometryFromPointCloud measures a cup from a cloud that already represents
// only the cup. MetaData().Center() is the bounding-box center ((max+min)/2),
// which is robust to point density -- a sparse / see-through cup still pins the
// extents, so the center and dimensions stay meaningful with few points.
func cupGeometryFromPointCloud(pc pointcloud.PointCloud) CupGeometry {
	md := pc.MetaData()
	return CupGeometry{
		Center:    md.Center(),
		RimZ:      md.MaxZ,
		Height:    md.MaxZ - md.MinZ,
		Width:     ((md.MaxX - md.MinX) + (md.MaxY - md.MinY)) / 2,
		NumPoints: pc.Size(),
	}
}

func (g CupGeometry) String() string {
	return fmt.Sprintf("CupGeometry(center=%v rimZ=%.1f height=%.1f width=%.1f points=%d)",
		g.Center, g.RimZ, g.Height, g.Width, g.NumPoints)
}

// CupSpec is the "standardized cup" the demo validates a detected cup against.
// Every field is optional: the accessor methods below supply defaults so a
// minimal config still works, and any value can be overridden in config.
//
// This replaces the old approach of matching detections to exact configured
// dimensions (cup_height/cup_width) within a fixed tolerance, which existed to
// pick the right blob out of a noisy multi-object segmentation. The new camera
// returns a single cup-only cloud, so instead we measure the cup and check it
// is (a) dense enough to trust, (b) a sensible size for the demo, and (c)
// something the gripper can physically close around.
type CupSpec struct {
	MinHeight float64 `json:"min_height"`
	MaxHeight float64 `json:"max_height"`
	MinWidth  float64 `json:"min_width"`
	MaxWidth  float64 `json:"max_width"`

	// GripperMaxOpening is the gripper's physical maximum span (mm). When 0 the
	// gripper-wrap check is skipped (default), so a guessed value never rejects
	// a cup -- set it to enforce that the claws can actually close around the cup.
	GripperMaxOpening float64 `json:"gripper_max_opening"`

	// GripClearance is the margin (mm) the gripper needs beyond the measured cup
	// width to wrap around it. Also used to inflate the cup collision obstacle.
	GripClearance float64 `json:"grip_clearance"`

	// MinPoints rejects clouds too sparse for the bounding box to be trusted.
	MinPoints int `json:"min_points"`

	// NominalHeight is a fallback cup height (mm) used to derive a release height
	// before any cup has been measured this run (see VinoCart.releaseZ).
	NominalHeight float64 `json:"nominal_height"`
}

// Permissive defaults: ranges are wide so the demo isn't surprised into
// rejecting a valid cup. Tighten them in config for stricter validation.
const (
	defaultCupMinHeight     = 30.0
	defaultCupMaxHeight     = 400.0
	defaultCupMinWidth      = 20.0
	defaultCupMaxWidth      = 200.0
	defaultCupGripClearance = 10.0
	defaultCupMinPoints     = 20
	defaultCupNominalHeight = 120.0
)

func (s *CupSpec) minHeight() float64 {
	if s.MinHeight > 0 {
		return s.MinHeight
	}
	return defaultCupMinHeight
}

func (s *CupSpec) maxHeight() float64 {
	if s.MaxHeight > 0 {
		return s.MaxHeight
	}
	return defaultCupMaxHeight
}

func (s *CupSpec) minWidth() float64 {
	if s.MinWidth > 0 {
		return s.MinWidth
	}
	return defaultCupMinWidth
}

func (s *CupSpec) maxWidth() float64 {
	if s.MaxWidth > 0 {
		return s.MaxWidth
	}
	return defaultCupMaxWidth
}

func (s *CupSpec) gripClearance() float64 {
	if s.GripClearance > 0 {
		return s.GripClearance
	}
	return defaultCupGripClearance
}

func (s *CupSpec) minPoints() int {
	if s.MinPoints > 0 {
		return s.MinPoints
	}
	return defaultCupMinPoints
}

func (s *CupSpec) nominalHeight() float64 {
	if s.NominalHeight > 0 {
		return s.NominalHeight
	}
	return defaultCupNominalHeight
}

// CupValidation is the result of checking a measured cup against the CupSpec.
// Reasons lists every failed check, so callers (and the UI/logs) can see why a
// cup was rejected rather than just a pass/fail bool.
type CupValidation struct {
	Valid   bool
	Reasons []string
}

func (s *CupSpec) validate(g CupGeometry) CupValidation {
	v := CupValidation{Valid: true}
	fail := func(format string, args ...interface{}) {
		v.Valid = false
		v.Reasons = append(v.Reasons, fmt.Sprintf(format, args...))
	}

	if g.NumPoints < s.minPoints() {
		fail("too few points (%d < %d): cloud not trustworthy", g.NumPoints, s.minPoints())
	}
	if g.Height < s.minHeight() || g.Height > s.maxHeight() {
		fail("height %.1f outside [%.1f, %.1f]", g.Height, s.minHeight(), s.maxHeight())
	}
	if g.Width < s.minWidth() || g.Width > s.maxWidth() {
		fail("width %.1f outside [%.1f, %.1f]", g.Width, s.minWidth(), s.maxWidth())
	}
	if opening := s.GripperMaxOpening; opening > 0 && g.Width+s.gripClearance() > opening {
		fail("width %.1f + clearance %.1f exceeds gripper opening %.1f", g.Width, s.gripClearance(), opening)
	}
	return v
}
