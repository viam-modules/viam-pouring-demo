package pour

import (
	"fmt"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/gripper"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/components/switch"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/services/vision"
)

type ConfigStatePostions map[string][][]string

func (csp ConfigStatePostions) All() []string {
	x := []string{}
	for _, aa := range csp {
		for _, bb := range aa {
			x = append(x, bb...)
		}
	}
	return x
}

func (csp ConfigStatePostions) setup(deps resource.Dependencies) (StagePositions, error) {
	m := map[string][][]toggleswitch.Switch{}
	for k, s := range csp { // stage name
		a := [][]toggleswitch.Switch{}
		for _, aa := range s { // parallel set 1
			b := []toggleswitch.Switch{}
			for _, bb := range aa { // actual pos
				sss, err := toggleswitch.FromDependencies(deps, bb)
				if err != nil {
					return nil, err
				}
				b = append(b, sss)
			}
			a = append(a, b)
		}

		m[k] = a
	}
	return m, nil
}

type Config struct {
	// dependencies, required
	ArmName                string `json:"arm_name"`
	CameraName             string `json:"camera_name"`
	CircleDetectionService string `json:"circle_detection_service"`
	WeightSensorName       string `json:"weight_sensor_name"`
	GripperName            string `json:"gripper_name"`

	GlassPourCam             string  `json:"glass_pour_cam"`
	GlassPourMotionThreshold float64 `json:"glass_pour_motion_threshold"`

	CupFinderService string `json:"cup_finder_service"`
	CupTopService    string `json:"cup_top_service"`

	Positions map[string]ConfigStatePostions

	BottleGripper string `json:"bottle_gripper"`
	BottleArm     string `json:"bottle_arm"`

	Handoff bool

	// cup and bottle params, required
	BottleHeight float64 `json:"bottle_height"`
	CupHeight    float64 `json:"cup_height"`
	DeltaXPos    float64 `json:"deltaxpos"`
	DeltaYPos    float64 `json:"deltaypos"`
	DeltaXNeg    float64 `json:"deltaxneg"`
	DeltaYNeg    float64 `json:"deltayneg"`

	BottleMotionService string `json:"bottle_motion_service"`
	CupMotionService    string `json:"cup_motion_service"`

	SimoneHack bool `json:"simone_hack"`
	Loop       bool `json:"loop"`

	// optional
	CPUThreads int `json:"cpu_threads,omitempty"`
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	deps := []string{motion.Named("builtin").String()}

	if cfg.BottleMotionService != "" {
		deps = append(deps, cfg.BottleMotionService)
	}

	if cfg.CupMotionService != "" {
		deps = append(deps, cfg.CupMotionService)
	}

	if cfg.ArmName == "" {
		return nil, nil, fmt.Errorf("need an arm name")
	}
	deps = append(deps, cfg.ArmName)

	if cfg.GripperName == "" {
		return nil, nil, fmt.Errorf("need a gripper name")
	}
	deps = append(deps, cfg.GripperName)

	if cfg.CameraName == "" {
		return nil, nil, fmt.Errorf("need a camera name")
	}
	deps = append(deps, cfg.CameraName)

	if cfg.BottleHeight == 0 {
		return nil, nil, fmt.Errorf("bottle_height cannot be unset")
	}
	if cfg.CupHeight == 0 {
		return nil, nil, fmt.Errorf("cup_height cannot be unset")
	}

	optionals := []string{}

	if cfg.CupFinderService != "" {
		optionals = append(optionals, cfg.CupFinderService)
	}
	if cfg.CupTopService != "" {
		optionals = append(optionals, cfg.CupTopService)
	}
	if cfg.WeightSensorName != "" {
		deps = append(deps, cfg.WeightSensorName)
	}
	if cfg.CircleDetectionService != "" {
		deps = append(deps, cfg.CircleDetectionService)
	}

	if cfg.BottleGripper != "" {
		deps = append(deps, cfg.BottleGripper)
	}
	if cfg.BottleArm != "" {
		deps = append(deps, cfg.BottleArm)
	}

	for _, ps := range cfg.Positions {
		deps = append(deps, ps.All()...)
	}

	if cfg.GlassPourCam != "" {
		deps = append(deps, cfg.GlassPourCam)
	}

	return deps, optionals, nil
}

func (c *Config) glassPourMotionThreshold() float64 {
	if c.GlassPourMotionThreshold > 0 {
		return c.GlassPourMotionThreshold
	}
	return 4
}

type StagePositions map[string][][]toggleswitch.Switch

type Pour1Components struct {
	Arm          arm.Arm
	Gripper      gripper.Gripper
	Cam          camera.Camera
	GlassPourCam camera.Camera
	Weight       sensor.Sensor
	Motion       motion.Service
	CamVision    vision.Service

	CupFinder vision.Service
	CupTop    vision.Service

	Positions map[string]StagePositions

	BottleGripper gripper.Gripper
	BottleArm     arm.Arm

	BottleMotionService motion.Service
	CupMotionService    motion.Service
}

func Pour1ComponentsFromDependencies(config *Config, deps resource.Dependencies) (*Pour1Components, error) {
	var err error
	c := &Pour1Components{}

	c.Arm, err = arm.FromDependencies(deps, config.ArmName)
	if err != nil {
		return nil, err
	}

	c.Gripper, err = gripper.FromDependencies(deps, config.GripperName)
	if err != nil {
		return nil, err
	}

	c.Cam, err = camera.FromDependencies(deps, config.CameraName)
	if err != nil {
		return nil, err
	}

	if config.GlassPourCam != "" {
		c.GlassPourCam, err = camera.FromDependencies(deps, config.GlassPourCam)
		if err != nil {
			return nil, err
		}
	}

	if config.WeightSensorName != "" {
		c.Weight, err = sensor.FromDependencies(deps, config.WeightSensorName)
		if err != nil {
			return nil, err
		}
	}

	c.Motion, err = motion.FromDependencies(deps, "builtin")
	if err != nil {
		return nil, err
	}

	if config.BottleMotionService != "" {
		c.BottleMotionService, err = motion.FromDependencies(deps, config.BottleMotionService)
		if err != nil {
			return nil, err
		}
	}

	if config.CupMotionService != "" {
		c.CupMotionService, err = motion.FromDependencies(deps, config.CupMotionService)
		if err != nil {
			return nil, err
		}
	}

	if config.CircleDetectionService != "" {
		c.CamVision, err = vision.FromDependencies(deps, config.CircleDetectionService)
		if err != nil {
			return nil, err
		}
	}

	if config.CupFinderService != "" {
		c.CupFinder, err = vision.FromDependencies(deps, config.CupFinderService)
		if err != nil {
			return nil, err
		}
	}

	if config.CupTopService != "" {
		c.CupTop, err = vision.FromDependencies(deps, config.CupTopService)
		if err != nil {
			return nil, err
		}
	}

	if config.BottleGripper != "" {
		c.BottleGripper, err = gripper.FromDependencies(deps, config.BottleGripper)
		if err != nil {
			return nil, err
		}
	}

	if config.BottleArm != "" {
		c.BottleArm, err = arm.FromDependencies(deps, config.BottleArm)
		if err != nil {
			return nil, err
		}
	}

	c.Positions = map[string]StagePositions{}
	for k, v := range config.Positions {
		ps, err := v.setup(deps)
		if err != nil {
			return nil, err
		}
		c.Positions[k] = ps
	}

	return c, nil
}
