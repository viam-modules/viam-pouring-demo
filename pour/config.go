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

type Config struct {
	// dependencies, required
	ArmName                string `json:"arm_name"`
	CameraName             string `json:"camera_name"`
	CircleDetectionService string `json:"circle_detection_service"`
	WeightSensorName       string `json:"weight_sensor_name"`
	GripperName            string `json:"gripper_name"`

	CupFinderService string `json:"cup_finder_service"`
	CupTopService    string `json:"cup_top_service"`

	RightBottlePourPreGrabActions  []string `json:"right_bottle_pour_pre_grab_actions"`
	RightBottlePourPostGrabActions []string `json:"right_bottle_pour_post_grab_actions"`
	BottleGripper                  string   `json:"bottle_gripper"`
	BottleArm                      string   `json:"bottle_arm"`

	LeftPlace   string `json:"left_place"`
	LeftRetreat string `json:"left_retreat"`

	// cup and bottle params, required
	BottleHeight float64 `json:"bottle_height"`
	CupHeight    float64 `json:"cup_height"`
	DeltaXPos    float64 `json:"deltaxpos"`
	DeltaYPos    float64 `json:"deltaypos"`
	DeltaXNeg    float64 `json:"deltaxneg"`
	DeltaYNeg    float64 `json:"deltayneg"`

	PourGlassFullnessService string `json:"pour_glass_fullness_service"`

	SimoneHack bool `json:"simone_hack"`

	// optional
	CPUThreads int `json:"cpu_threads,omitempty"`
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	deps := []string{motion.Named("builtin").String()}

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

	if cfg.LeftPlace != "" {
		deps = append(deps, cfg.LeftPlace)
	}
	if cfg.LeftRetreat != "" {
		deps = append(deps, cfg.LeftRetreat)
	}

	if cfg.PourGlassFullnessService != "" {
		deps = append(deps, cfg.PourGlassFullnessService)
	}

	deps = append(deps, cfg.RightBottlePourPreGrabActions...)
	deps = append(deps, cfg.RightBottlePourPostGrabActions...)
	return deps, optionals, nil
}

type Pour1Components struct {
	Arm       arm.Arm
	Gripper   gripper.Gripper
	Cam       camera.Camera
	Weight    sensor.Sensor
	Motion    motion.Service
	CamVision vision.Service

	CupFinder                vision.Service
	CupTop                   vision.Service
	PourGlassFullnessService vision.Service

	RightBottlePourPreGrabActions  []toggleswitch.Switch
	RightBottlePourPostGrabActions []toggleswitch.Switch
	BottleGripper                  gripper.Gripper
	BottleArm                      arm.Arm

	LeftPlace, LeftRetreat toggleswitch.Switch
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

	if config.PourGlassFullnessService != "" {
		c.PourGlassFullnessService, err = vision.FromDependencies(deps, config.PourGlassFullnessService)
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

	for _, x := range config.RightBottlePourPreGrabActions {
		s, err := toggleswitch.FromDependencies(deps, x)
		if err != nil {
			return nil, err
		}
		c.RightBottlePourPreGrabActions = append(c.RightBottlePourPreGrabActions, s)
	}

	for _, x := range config.RightBottlePourPostGrabActions {
		s, err := toggleswitch.FromDependencies(deps, x)
		if err != nil {
			return nil, err
		}
		c.RightBottlePourPostGrabActions = append(c.RightBottlePourPostGrabActions, s)
	}

	if config.LeftRetreat != "" {
		c.LeftRetreat, err = toggleswitch.FromDependencies(deps, config.LeftRetreat)
		if err != nil {
			return nil, err
		}
	}

	if config.LeftPlace != "" {
		c.LeftPlace, err = toggleswitch.FromDependencies(deps, config.LeftPlace)
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}
