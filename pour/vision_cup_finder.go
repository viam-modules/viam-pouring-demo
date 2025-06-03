package pour

import (
	"context"
	"fmt"
	"image"
	"math"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	viz "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/classification"
	"go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/rdk/vision/viscapture"
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
	Input       string
	HeightMM    float64 `json:"height_mm"`
	RadiusMM    float64 `json:"radius_mm"`
	ErrorMargin float64 `json:"error_margin"`
}

func (c *VisionCupFinderConfig) Validate(_ string) ([]string, []string, error) {
	if c.Input == "" {
		return nil, nil, fmt.Errorf("need input")
	}
	if c.HeightMM <= 0 {
		return nil, nil, fmt.Errorf("need height_mm")
	}
	if c.RadiusMM <= 0 {
		return nil, nil, fmt.Errorf("need radius_mm")
	}
	if c.ErrorMargin <= 0 {
		return nil, nil, fmt.Errorf("need error_margin")
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

func (vcf *visionCupFinder) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*viz.Object, error) {
	os, err := vcf.input.GetObjectPointClouds(ctx, cameraName, extra)
	if err != nil {
		return nil, err
	}

	return os, nil
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

func cleanPointCloud(pc pointcloud.PointCloud) (pointcloud.PointCloud, error) {
	temp := pointcloud.NewBasicEmpty()
	f, err := pointcloud.StatisticalOutlierFilter(200, 1.5)
	if err != nil {
		return nil, err
	}

	err = f(pc, temp)
	if err != nil {
		return nil, err
	}
	return temp, nil
}

func FilterObjects(objects []*viz.Object, correctHeight, correctWidth, goodDelta float64, logger logging.Logger) []*viz.Object {
	good := []*viz.Object{}

	for idx, o := range objects {
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
