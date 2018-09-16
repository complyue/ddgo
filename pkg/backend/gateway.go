package backend

import (
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/gorilla/mux"
)

// this var can be replaced to facilitate alternative service discovery mechanism
var GetRoutesService = func(tunnel, session string) (*routes.ConsumerAPI, error) {
	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	return svcs.GetRoutesService(
		tunnel, session,
	)
}

func DefineHttpRoutes(router *mux.Router) {

	router.HandleFunc("/api/{tid}/waypoint", showWaypoints)
	router.HandleFunc("/api/{tid}/waypoint/add", addWaypoint)
	router.HandleFunc("/api/{tid}/waypoint/move", moveWaypoint)

	/*
		router.add_get('/api/{tid}/truck', show_trucks)
		router.add_post('/api/{tid}/truck/add', add_truck)
		router.add_post('/api/{tid}/truck/move', move_truck)
		router.add_post('/api/{tid}/truck/stop', stop_truck)
	*/

}
