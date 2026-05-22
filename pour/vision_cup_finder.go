package pour

import (
	"context"
	"fmt"
	"image"
	"math"

	"github.com/golang/geo/r3"
	commonpb "go.viam.com/api/common/v1"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/spatialmath"
	viz "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/classification"
	"go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/rdk/vision/viscapture"
)

// CupDetectionMetaLabel marks the first GetObjectPointClouds object carrying
// the service summary. The frontend reads it to render expected cup dimensions
// and the valid/invalid counters in the 3D viewer.
// Box dimensions (mm): X=expected cup height, Y=expected cup width, Z=tolerance (good_delta).
// Box center pose: X=total cup objects, Y=valid cups, Z=invalid cups.
const CupDetectionMetaLabel = "__cup_detection_meta__"

const (
	cupLabelValid   = "cup_valid"
	cupLabelInvalid = "cup_invalid"
)

var VisionCupFinderModel = NamespaceFamily.WithModel("vision-cup-finder")

func init() {
	resource.RegisterService(
		vision.API,
		VisionCupFinderModel,
		resource.Registration[vision.Service, *VisionCupFinderConfig]{
			Constructor: newVisionCupFinder,
		})
}

// VisionCupFinderConfig configures the diagnostic cup finder.
//
// Input is the name of a camera whose NextPointCloud returns the single
// already-segmented cup (e.g. a SAM2 merged-cup camera). HeightMM and WidthMM
// are the expected cup dimensions; GoodDelta is the per-axis tolerance used to
// decide cup_valid vs cup_invalid. MaxPoints downsamples the returned point
// cloud for frontend transport.
type VisionCupFinderConfig struct {
	Input     string  `json:"input"`
	HeightMM  float64 `json:"height_mm"`
	WidthMM   float64 `json:"width_mm"`
	GoodDelta float64 `json:"good_delta"`
	MaxPoints int     `json:"max_points"`
}

func (c *VisionCupFinderConfig) Validate(_ string) ([]string, []string, error) {
	if c.Input == "" {
		return nil, nil, fmt.Errorf("need input")
	}
	if c.HeightMM <= 0 {
		return nil, nil, fmt.Errorf("need height_mm")
	}
	if c.WidthMM <= 0 {
		return nil, nil, fmt.Errorf("need width_mm")
	}
	if c.GoodDelta <= 0 {
		return nil, nil, fmt.Errorf("need good_delta")
	}
	return []string{c.Input}, nil, nil
}

func newVisionCupFinder(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (vision.Service, error) {
	config, err := resource.NativeConfig[*VisionCupFinderConfig](conf)
	if err != nil {
		return nil, err
	}

	cf := &visionCupFinder{
		name:   conf.ResourceName(),
		cfg:    config,
		logger: logger,
	}

	cf.input, err = camera.FromProvider(deps, config.Input)
	if err != nil {
		return nil, err
	}

	return cf, nil
}

type visionCupFinder struct {
	resource.AlwaysRebuild
	resource.TriviallyCloseable

	name   resource.Name
	cfg    *VisionCupFinderConfig
	logger logging.Logger

	input camera.Camera
}

func (vcf *visionCupFinder) Name() resource.Name {
	return vcf.name
}

func (vcf *visionCupFinder) maxPoints() int {
	if vcf.cfg.MaxPoints > 0 {
		return vcf.cfg.MaxPoints
	}
	return 500
}

func (vcf *visionCupFinder) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*viz.Object, error) {
	cloud, err := vcf.input.NextPointCloud(ctx, nil)
	if err != nil {
		return nil, err
	}

	// No cup observed: still emit a meta summary so the frontend can update counters.
	if cloud == nil || cloud.Size() == 0 {
		metaObj, err := metaSummaryObject(vcf.cfg.HeightMM, vcf.cfg.WidthMM, vcf.cfg.GoodDelta, 0, 0, 0)
		if err != nil {
			return nil, err
		}
		return []*viz.Object{metaObj}, nil
	}

	obj, err := viz.NewObject(cloud)
	if err != nil {
		return nil, err
	}

	analysis := analyzeCup(obj, vcf.cfg.HeightMM, vcf.cfg.WidthMM, vcf.cfg.GoodDelta)
	label := cupLabelInvalid
	valid := 0
	invalid := 1
	if analysis.Valid {
		label = cupLabelValid
		valid = 1
		invalid = 0
	}

	if vcf.logger != nil {
		vcf.logger.Infof(
			"vision-cup-finder height: %0.2f (delta %0.2f, pass=%v) width: %0.2f (delta %0.2f, pass=%v)",
			analysis.Height, analysis.HeightDelta, analysis.HeightPass,
			analysis.Width, analysis.WidthDelta, analysis.WidthPass,
		)
	}

	enriched, err := enrichCupObject(obj, label, vcf.maxPoints())
	if err != nil {
		return nil, err
	}

	metaObj, err := metaSummaryObject(vcf.cfg.HeightMM, vcf.cfg.WidthMM, vcf.cfg.GoodDelta, 1, valid, invalid)
	if err != nil {
		return nil, err
	}

	return []*viz.Object{metaObj, enriched}, nil
}

func metaSummaryObject(cupHeight, cupWidth, goodDelta float64, total, valid, invalid int) (*viz.Object, error) {
	geom, err := spatialmath.NewBox(
		spatialmath.NewPose(
			r3.Vector{X: float64(total), Y: float64(valid), Z: float64(invalid)},
			spatialmath.NewZeroOrientation(),
		),
		r3.Vector{X: cupHeight, Y: cupWidth, Z: goodDelta},
		CupDetectionMetaLabel,
	)
	if err != nil {
		return nil, err
	}
	return viz.NewObjectWithLabel(pointcloud.NewBasicEmpty(), CupDetectionMetaLabel, geom.ToProtobuf())
}

func enrichCupObject(o *viz.Object, label string, maxPoints int) (*viz.Object, error) {
	total := o.Size()
	pc := downsamplePointCloud(o, total, maxPoints)
	var geomProto *commonpb.Geometry
	if o.Geometry != nil {
		geomProto = o.Geometry.ToProtobuf()
		if geomProto != nil {
			geomProto.Label = label
		}
	}
	return viz.NewObjectWithLabel(pc, label, geomProto)
}

func downsamplePointCloud(o *viz.Object, total, maxPoints int) pointcloud.PointCloud {
	if maxPoints <= 0 || total <= maxPoints {
		return o
	}
	step := total / maxPoints
	if step < 1 {
		step = 1
	}
	out := pointcloud.NewBasicEmpty()
	i := 0
	count := 0
	o.Iterate(0, 0, func(p r3.Vector, d pointcloud.Data) bool {
		if i%step == 0 {
			_ = out.Set(p, d)
			count++
		}
		i++
		return count < maxPoints
	})
	return out
}

func IsCupDetectionMetaObject(o *viz.Object) bool {
	return o != nil && o.Geometry != nil && o.Geometry.Label() == CupDetectionMetaLabel
}

func (vcf *visionCupFinder) DetectionsFromCamera(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
	return nil, fmt.Errorf("no detection support")
}

func (vcf *visionCupFinder) Detections(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
	return nil, fmt.Errorf("no detection support")
}

func (vcf *visionCupFinder) ClassificationsFromCamera(
	ctx context.Context,
	cameraName string,
	n int,
	extra map[string]interface{},
) (classification.Classifications, error) {
	return nil, fmt.Errorf("no classification support")
}

func (vcf *visionCupFinder) Classifications(
	ctx context.Context,
	img image.Image,
	n int,
	extra map[string]interface{},
) (classification.Classifications, error) {
	return nil, fmt.Errorf("no classification support")
}

func (vcf *visionCupFinder) GetProperties(ctx context.Context, extra map[string]interface{}) (*vision.Properties, error) {
	return &vision.Properties{
		ObjectPCDsSupported: true,
	}, nil
}

func (vcf *visionCupFinder) CaptureAllFromCamera(ctx context.Context, cameraName string, opts viscapture.CaptureOptions, extra map[string]interface{}) (viscapture.VisCapture, error) {
	res := viscapture.VisCapture{}
	if opts.ReturnObject {
		os, err := vcf.GetObjectPointClouds(ctx, cameraName, extra)
		if err != nil {
			return res, err
		}
		res.Objects = os
	}
	return res, nil
}

func (vcf *visionCupFinder) DoCommand(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}

// cupConstraintResult is the per-axis breakdown of a single cup's measured
// dimensions against the expected ones; used only by the diagnostic path.
type cupConstraintResult struct {
	Height      float64
	HeightDelta float64
	HeightPass  bool
	Width       float64
	WidthDelta  float64
	WidthPass   bool
	Valid       bool
}

func analyzeCup(o *viz.Object, correctHeight, correctWidth, goodDelta float64) cupConstraintResult {
	md := o.MetaData()
	height := md.MaxZ
	width := ((md.MaxY - md.MinY) + (md.MaxX - md.MinX)) / 2
	heightDelta := math.Abs(height - correctHeight)
	widthDelta := math.Abs(correctWidth - width)
	return cupConstraintResult{
		Height:      height,
		HeightDelta: heightDelta,
		HeightPass:  heightDelta <= goodDelta,
		Width:       width,
		WidthDelta:  widthDelta,
		WidthPass:   widthDelta <= goodDelta,
		Valid:       heightDelta <= goodDelta && widthDelta <= goodDelta,
	}
}
