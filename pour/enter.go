// Package pour implements a generic service to pour liquids into cups
package pour

import (
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
)

var Model = NamespaceFamily.WithModel("pour")

func init() {
	resource.RegisterService(generic.API, Model, resource.Registration[resource.Resource, *Config]{Constructor: newVinoCart})
}
