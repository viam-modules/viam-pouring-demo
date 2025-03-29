// Package main implements a module that pours liquids into cups
package main

import (
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"

	"github.com/viam-modules/viam-pouring-demo/pour"
)

func main() {
	module.ModularMain(
		resource.APIModel{API: generic.API, Model: pour.Model},
		resource.APIModel{API: sensor.API, Model: pour.WeightModel},
		resource.APIModel{API: sensor.API, Model: pour.WeightHardcodedModel},
	)
}
