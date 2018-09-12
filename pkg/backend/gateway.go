package backend

import (
	"github.com/kataras/iris"
)

func DefineHttpRoutes(app *iris.Application) {

	app.Get("/api/{tid:string}/waypoint", showWaypoints().Handler())
	app.Post("/api/{tid:string}/waypoint/add", addWaypoint)
	app.Post("/api/{tid:string}/waypoint/move", moveWaypoint)

	/*
		router.add_get('/api/{tid}/truck', show_trucks)
		router.add_post('/api/{tid}/truck/add', add_truck)
		router.add_post('/api/{tid}/truck/move', move_truck)
		router.add_post('/api/{tid}/truck/stop', stop_truck)
	*/

}
