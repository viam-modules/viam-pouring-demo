// Package main implements a module that pours liquids into cups
package main

import (
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
)

const moduleName = "Liquid Pouring Go Module"

var ModelName = resource.NewModel("viam", "viam-pouring-demo", "pour")

func main() {
	module.ModularMain(
		moduleName,
		resource.APIModel{API: generic.API, Model: ModelName},
	)

	// INFO	    rdk.modmanager	modmanager/manager.go:276	Now adding module	{"module":"local-module-1"}
	// INFO	    rdk.modmanager	modmanager/manager.go:357	Creating data directory "/Users/nick/.viam/module-data/7c2729a4-7bed-4699-a431-821abb26c468/local-module-1" for module "local-module-1"
	// WARN	    rdk.modmanager	modmanager/manager.go:1204	VIAM_MODULE_ROOT was not passed to module. Defaulting to module's working directory	{"module":"local-module-1","dir":"/Users/nick/Desktop/viam-pouring-demo/bin/darwin-arm64"}
	// INFO	    rdk.modmanager	modmanager/manager.go:1236	Starting up module	{"module":"local-module-1"}
	// INFO	    rdk.modmanager.process.local-module-1_/Users/nick/Desktop/viam-pouring-demo/bin/darwin-arm64/viam-pouring-demo.StdOut	pexec/managed_process.go:277
	// ERROR	Liquid Pouring Go Module	utils@v0.1.106/runtime.go:78	resource with API rdk:service:generic and model viam:viam-pouring-demo:pour not yet registered
	// INFO	    rdk.modmanager.process.local-module-1_/Users/nick/Desktop/viam-pouring-demo/bin/darwin-arm64/viam-pouring-demo	pexec/managed_process_unix.go:91	stopping process 18574 with signal terminated
	// ERROR	rdk.modmanager	modmanager/manager.go:1365	Error while stopping process of module that failed to start	{"module":"local-module-1","error":"exit status 1"}
	// ERROR	rdk.modmanager	modmanager/manager.go:279	Error adding module	{"module":"local-module-1","error":"error while starting module local-module-1: module local-module-1 exited too quickly after attempted startup; it might have a fatal runtime issue","errorVerbose":"module local-module-1 exited too quickly after attempted startup; it might have a fatal runtime issue\nerror while starting module local-module-1"}
	// ERROR	rdk.resource_manager	impl/resource_manager.go:1096	error adding modules	{"error":"error while starting module local-module-1: module local-module-1 exited too quickly after attempted startup; it might have a fatal runtime issue","errorVerbose":"module local-module-1 exited too quickly after attempted startup; it might have a fatal runtime issue\nerror while starting module local-module-1"}
}
