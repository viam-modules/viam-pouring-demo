package pour

import (
	"context"
	"fmt"
	"image"
	"math"

	"github.com/golang/geo/r3"
	commonpb "go.viam.com/api/common/v1"
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

// CupDetectionMetaLabel marks the first GetObjectPointClouds object carrying service summary.
// Config is encoded in the meta box dimensions (mm); detection counts in the box center pose.
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

	cf.input, err = vision.FromDependencies(deps, config.Input)
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

	input vision.Service
}

func (vcf *visionCupFinder) Name() resource.Name {
	return vcf.name
}

func (vcf *visionCupFinder) goodDelta() float64 {
	if vcf.cfg.GoodDelta > 0 {
		return vcf.cfg.GoodDelta
	}
	return 25
}

func (vcf *visionCupFinder) maxPoints() int {
	if vcf.cfg.MaxPoints > 0 {
		return vcf.cfg.MaxPoints
	}
	return 500
}

func (vcf *visionCupFinder) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*viz.Object, error) {
	objects, err := vcf.input.GetObjectPointClouds(ctx, cameraName, extra)
	if err != nil {
		return nil, err
	}

	goodDelta := vcf.goodDelta()
	validCups := FilterObjects(objects, vcf.cfg.HeightMM, vcf.cfg.WidthMM, goodDelta, vcf.logger)

	out := make([]*viz.Object, 0, len(objects)+1)
	for _, o := range objects {
		analysis := AnalyzeObject(o, vcf.cfg.HeightMM, vcf.cfg.WidthMM, goodDelta)
		label := cupLabelInvalid
		if analysis.Valid {
			label = cupLabelValid
		}
		enriched, err := enrichCupObject(o, label, vcf.maxPoints())
		if err != nil {
			return nil, err
		}
		out = append(out, enriched)
	}

	metaObj, err := metaSummaryObject(
		vcf.cfg.HeightMM,
		vcf.cfg.WidthMM,
		goodDelta,
		len(objects),
		len(validCups),
		len(objects)-len(validCups),
	)
	if err != nil {
		return nil, err
	}
	return append([]*viz.Object{metaObj}, out...), nil
}

// metaSummaryObject is the first object in GetObjectPointClouds responses.
// Box dimensions (mm): X=expected cup height, Y=expected cup width, Z=tolerance (good_delta).
// Box center position: X=total cup objects, Y=valid cups, Z=invalid cups.
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

type CupConstraintResult struct {
	Height      float64
	ExpHeight   float64
	HeightDelta float64
	HeightPass  bool
	Width       float64
	ExpWidth    float64
	WidthDelta  float64
	WidthPass   bool
	Valid       bool
	GoodDelta   float64
}

func AnalyzeObject(o *viz.Object, correctHeight, correctWidth, goodDelta float64) CupConstraintResult {
	md := o.MetaData()
	height := md.MaxZ
	width := ((md.MaxY - md.MinY) + (md.MaxX - md.MinX)) / 2
	heightDelta := math.Abs(height - correctHeight)
	widthDelta := math.Abs(correctWidth - width)
	return CupConstraintResult{
		Height:      height,
		ExpHeight:   correctHeight,
		HeightDelta: heightDelta,
		HeightPass:  heightDelta <= goodDelta,
		Width:       width,
		ExpWidth:    correctWidth,
		WidthDelta:  widthDelta,
		WidthPass:   widthDelta <= goodDelta,
		Valid:       heightDelta <= goodDelta && widthDelta <= goodDelta,
		GoodDelta:   goodDelta,
	}
}

func FilterObjects(objects []*viz.Object, correctHeight, correctWidth, goodDelta float64, logger logging.Logger) []*viz.Object {
	good := []*viz.Object{}

	for idx, o := range objects {
		if IsCupDetectionMetaObject(o) {
			continue
		}

		md := o.MetaData()

		height := md.MaxZ
		width := ((md.MaxY - md.MinY) + (md.MaxX - md.MinX)) / 2

		heightDelta := math.Abs(height - correctHeight)
		widthDelta := math.Abs(correctWidth - width)

		if logger != nil {
			logger.Infof("FindCups %d %v height: %0.2f heightDelta: %0.2f (%v) width: %0.2f widthDelta: %0.2f (%v)",
				idx, o,
				height, heightDelta, heightDelta <= goodDelta,
				width, widthDelta, widthDelta <= goodDelta,
			)
		}

		if heightDelta > goodDelta || widthDelta > goodDelta {
			continue
		}

		good = append(good, o)
	}

	return good
}
