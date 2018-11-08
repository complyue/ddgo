package backend

import (
	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/gorilla/mux"
)

// this var can be replaced to facilitate alternative service discovery mechanism
var GetRoutesService = func(tid string) (*routes.ConsumerAPI, error) {
	return routes.GetRoutesService(tid)
}

// this var can be replaced to facilitate alternative service discovery mechanism
var GetDriversService = func(tid string) (*drivers.ConsumerAPI, error) {
	return drivers.GetDriversService(tid)
}

func DefineHttpRoutes(router *mux.Router) {

	// router.HandleFunc("/api/{tid}/auth", authenticateUser)
	// router.HandleFunc("/api/{tid}/register", registerUser)

	router.HandleFunc("/api/{tid}/waypoint", showWaypoints)
	router.HandleFunc("/api/{tid}/waypoint/add", addWaypoint)
	router.HandleFunc("/api/{tid}/waypoint/move", moveWaypoint)

	router.HandleFunc("/api/{tid}/truck", showTrucks)
	router.HandleFunc("/api/{tid}/truck/add", addTruck)
	router.HandleFunc("/api/{tid}/truck/move", moveTruck)
	router.HandleFunc("/api/{tid}/truck/stop", stopTruck)

}
