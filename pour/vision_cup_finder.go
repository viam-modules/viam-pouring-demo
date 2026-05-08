package pour

import (
	"context"
	"fmt"
	"math"

	"github.com/golang/geo/r3"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	viz "go.viam.com/rdk/vision"
)

var CupDetectionSensorModel = NamespaceFamily.WithModel("cup-detection-sensor")

func init() {
	resource.RegisterComponent(
		sensor.API,
		CupDetectionSensorModel,
		resource.Registration[sensor.Sensor, *CupDetectionSensorConfig]{
			Constructor: newCupDetectionSensor,
		})
}

type CupDetectionSensorConfig struct {
	Input      string  `json:"input"`
	HeightMM   float64 `json:"height_mm"`
	WidthMM    float64 `json:"width_mm"`
	GoodDelta  float64 `json:"good_delta"`
	MaxPoints  int     `json:"max_points"`
}

func (c *CupDetectionSensorConfig) Validate(_ string) ([]string, []string, error) {
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

func newCupDetectionSensor(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (sensor.Sensor, error) {
	config, err := resource.NativeConfig[*CupDetectionSensorConfig](conf)
	if err != nil {
		return nil, err
	}

	cf := &cupDetectionSensor{
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

type cupDetectionSensor struct {
	resource.AlwaysRebuild
	resource.TriviallyCloseable

	name   resource.Name
	cfg    *CupDetectionSensorConfig
	logger logging.Logger

	input vision.Service
}

func (vcf *cupDetectionSensor) Name() resource.Name {
	return vcf.name
}

func (vcf *cupDetectionSensor) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	objects, err := vcf.input.GetObjectPointClouds(ctx, "", nil)
	if err != nil {
		return nil, err
	}

	goodDelta := vcf.cfg.GoodDelta
	if goodDelta <= 0 {
		goodDelta = 25
	}
	validCups := FilterObjects(objects, vcf.cfg.HeightMM, vcf.cfg.WidthMM, goodDelta, nil)
	cupDetails := BuildCupDetails(objects, vcf.cfg.HeightMM, vcf.cfg.WidthMM, goodDelta, vcf.maxPoints())

	return map[string]interface{}{
		"cup_height": vcf.cfg.HeightMM,
		"cup_width":  vcf.cfg.WidthMM,
		"detection": map[string]interface{}{
			"total_cup_objects": len(objects),
			"valid_cups":        len(validCups),
			"invalid_cups":      len(objects) - len(validCups),
		},
		"cups": cupDetails,
	}, nil
}

func (vcf *cupDetectionSensor) maxPoints() int {
	if vcf.cfg.MaxPoints > 0 {
		return vcf.cfg.MaxPoints
	}
	return 500
}

func (vcf *cupDetectionSensor) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
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

func BuildCupDetails(objects []*viz.Object, correctHeight, correctWidth, goodDelta float64, maxPoints int) []interface{} {
	cups := make([]interface{}, 0, len(objects))
	for idx, o := range objects {
		analysis := AnalyzeObject(o, correctHeight, correctWidth, goodDelta)

		pointsX := make([]interface{}, 0)
		pointsY := make([]interface{}, 0)
		pointsZ := make([]interface{}, 0)

		total := o.Size()
		step := 1
		if maxPoints > 0 && total > maxPoints {
			step = total / maxPoints
		}

		i := 0
		count := 0
		o.Iterate(0, 0, func(p r3.Vector, d pointcloud.Data) bool {
			if i%step == 0 {
				pointsX = append(pointsX, p.X)
				pointsY = append(pointsY, p.Y)
				pointsZ = append(pointsZ, p.Z)
				count++
			}
			i++
			return maxPoints <= 0 || count < maxPoints
		})

		cups = append(cups, map[string]interface{}{
			"index":           idx,
			"valid":           analysis.Valid,
			"height":          analysis.Height,
			"expected_height": analysis.ExpHeight,
			"height_delta":    analysis.HeightDelta,
			"height_pass":     analysis.HeightPass,
			"width":           analysis.Width,
			"expected_width":  analysis.ExpWidth,
			"width_delta":     analysis.WidthDelta,
			"width_pass":      analysis.WidthPass,
			"good_delta":      analysis.GoodDelta,
			"total_points":    total,
			"points_x":        pointsX,
			"points_y":        pointsY,
			"points_z":        pointsZ,
		})
	}
	return cups
}
