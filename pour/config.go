package pour

import (
	"fmt"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/gripper"
	toggleswitch "go.viam.com/rdk/components/switch"
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
	ArmName     string `json:"arm_name"`
	CameraName  string `json:"camera_name"`
	GripperName string `json:"gripper_name"`

	GlassPourCam             string  `json:"glass_pour_cam"`
	GlassPourMotionThreshold float64 `json:"glass_pour_motion_threshold"`

	CupFinderService    string `json:"cup_finder_service"`    // find the cups on the table
	BottleFinderService string `json:"bottle_finder_service"` // find the bottle on the table

	Positions map[string]ConfigStatePostions

	BottleGripper string `json:"bottle_gripper"`
	BottleArm     string `json:"bottle_arm"`

	Handoff bool

	// cup and bottle params, required
	BottleHeight              float64 `json:"bottle_height"`
	BottleFindHeight          float64 `json:"bottle_find_height"`
	BottleWidth               float64 `json:"bottle_width"`
	BottleGripHeight          float64 `json:"bottle_grip_height"`
	CupHeight                 float64 `json:"cup_height"`
	CupWidth                  float64 `json:"cup_width"`
	GripperToBottleCenterHack float64 `json:"gripper_to_bottle_center_hack"`

	// optional offset for gripper height when grabbing/placing cup
	CupGripHeightOffset float64 `json:"cup_grip_height_offset"`

	PickQualityService   string `json:"pick_quality_service"`
	PourGlassFindService string `json:"pour_glass_find_service"`

	Loop bool `json:"loop"`
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	deps := []string{motion.Named("builtin").String()}

	if cfg.PickQualityService != "" {
		deps = append(deps, cfg.PickQualityService)
	}

	if cfg.PourGlassFindService != "" {
		deps = append(deps, cfg.PourGlassFindService)
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
	if cfg.BottleFindHeight == 0 {
		return nil, nil, fmt.Errorf(("bottle_find_height cannot be unset"))
	}
	if cfg.BottleGripHeight == 0 {
		return nil, nil, fmt.Errorf("bottle_grip_heigh cannot be unset")
	}
	if cfg.CupHeight == 0 {
		return nil, nil, fmt.Errorf("cup_height cannot be unset")
	}
	if cfg.GripperToBottleCenterHack == 0 {
		return nil, nil, fmt.Errorf(("gripper_to_bottle_center_hack cannot be unset"))
	}

	optionals := []string{}

	if cfg.CupFinderService != "" {
		optionals = append(optionals, cfg.CupFinderService)
	}

	if cfg.BottleFinderService != "" {
		optionals = append(optionals, cfg.BottleFinderService)
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

func (c *Config) cupWidth() float64 {
	if c.CupWidth > 0 {
		return c.CupWidth
	}
	return c.CupHeight * .6
}

func (c *Config) glassPourMotionThreshold() float64 {
	if c.GlassPourMotionThreshold > 0 {
		return c.GlassPourMotionThreshold
	}
	return 4
}

func (c *Config) cupGripHeightOffset() float64 {
	if c.CupGripHeightOffset > 0 {
		return c.CupGripHeightOffset
	}
	return 25
}

type StagePositions map[string][][]toggleswitch.Switch

type Pour1Components struct {
	Arm          arm.Arm
	Gripper      gripper.Gripper
	Cam          camera.Camera
	GlassPourCam camera.Camera
	Motion       motion.Service
	CamVision    vision.Service

	CupFinder    vision.Service
	BottleFinder vision.Service

	Positions map[string]StagePositions

	BottleGripper gripper.Gripper
	BottleArm     arm.Arm

	PickQualityService   vision.Service
	PourGlassFindService vision.Service
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

	c.Motion, err = motion.FromDependencies(deps, "builtin")
	if err != nil {
		return nil, err
	}

	if config.PickQualityService != "" {
		c.PickQualityService, err = vision.FromDependencies(deps, config.PickQualityService)
		if err != nil {
			return nil, err
		}
	}

	if config.PourGlassFindService != "" {
		c.PourGlassFindService, err = vision.FromDependencies(deps, config.PourGlassFindService)
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

	if config.BottleFinderService != "" {
		c.BottleFinder, err = vision.FromDependencies(deps, config.BottleFinderService)
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
