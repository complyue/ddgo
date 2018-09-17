package drivers

import (
	"github.com/complyue/ddgo/pkg/routes"
)

// this var can be replaced to facilitate alternative service discovery mechanism
var GetRoutesService = func(tunnel, session string) (*routes.ConsumerAPI, error) {
	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	return routes.GetRoutesService(
		tunnel, session,
	)
}
