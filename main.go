// Package main implements a module that pours liquids into cups
package main

import (
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"main.go/pour"
)

const moduleName = "Liquid Pouring Go Module"

func main() {
	module.ModularMain(
		// moduleName,
		resource.APIModel{API: generic.API, Model: pour.GenericServiceName},
	)
}
