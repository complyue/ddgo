package drivers

import (
	"github.com/complyue/ddgo/pkg/routes"
)

// this var can be replaced to facilitate alternative service discovery mechanism
var GetRoutesService = func(tid string) (*routes.ConsumerAPI, error) {
	return routes.GetRoutesService(tid)
}
