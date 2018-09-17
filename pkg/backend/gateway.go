package backend

import (
	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/gorilla/mux"
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

// this var can be replaced to facilitate alternative service discovery mechanism
var GetDriversService = func(tunnel, session string) (*drivers.ConsumerAPI, error) {
	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	return drivers.GetDriversService(
		tunnel, session,
	)
}

func DefineHttpRoutes(router *mux.Router) {

	router.HandleFunc("/api/{tid}/waypoint", showWaypoints)
	router.HandleFunc("/api/{tid}/waypoint/add", addWaypoint)
	router.HandleFunc("/api/{tid}/waypoint/move", moveWaypoint)

	router.HandleFunc("/api/{tid}/truck", showTrucks)
	router.HandleFunc("/api/{tid}/truck/add", addTruck)
	router.HandleFunc("/api/{tid}/truck/move", moveTruck)
	router.HandleFunc("/api/{tid}/truck/stop", stopTruck)

}
