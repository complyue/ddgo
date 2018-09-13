package backend

import (
	"github.com/gorilla/mux"
)

func DefineHttpRoutes(router *mux.Router) {

	router.HandleFunc("/api/{tid}/waypoint", showWaypoints)
	router.HandleFunc("/api/{tid}/waypoint/add", addWaypoint)
	router.HandleFunc("/api/{tid}/waypoint/move", moveWaypoint)

	/*
		app.Get("/api/{tid:string}/waypoint", showWaypoints().Handler())
		app.Post("/api/{tid:string}/waypoint/add", addWaypoint)
		app.Post("/api/{tid:string}/waypoint/move", moveWaypoint)

			router.add_get('/api/{tid}/truck', show_trucks)
			router.add_post('/api/{tid}/truck/add', add_truck)
			router.add_post('/api/{tid}/truck/move', move_truck)
			router.add_post('/api/{tid}/truck/stop', stop_truck)
	*/

}
