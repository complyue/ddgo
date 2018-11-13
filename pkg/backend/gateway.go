package backend

import (
	"sync"

	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/gorilla/mux"
)

var (
	// this var can be replaced to facilitate alternative service discovery mechanism
	InitRoutesService = func(tid string) (*routes.ConsumerAPI, error) {
		return routes.NewConsumerAPI(tid), nil
	}

	routesAPIs map[string]*routes.ConsumerAPI
	muRoutes   sync.Mutex
)

func GetRoutesService(tid string) (*routes.ConsumerAPI, error) {
	muRoutes.Lock()
	defer muRoutes.Unlock()

	if routesAPIs != nil {
		if routesAPI, ok := routesAPIs[tid]; ok {
			return routesAPI, nil
		}
	} else {
		routesAPIs = make(map[string]*routes.ConsumerAPI)
	}

	routesAPI, err := InitRoutesService(tid)
	if err != nil {
		return nil, err
	}

	routesAPIs[tid] = routesAPI

	return routesAPI, nil
}

var (
	// this var can be replaced to facilitate alternative service discovery mechanism
	InitDriversService = func(tid string) (*drivers.ConsumerAPI, error) {
		return drivers.NewConsumerAPI(tid), nil
	}

	driversAPIs map[string]*drivers.ConsumerAPI
	muDrivers   sync.Mutex
)

func GetDriversService(tid string) (*drivers.ConsumerAPI, error) {
	muDrivers.Lock()
	defer muDrivers.Unlock()

	if driversAPIs != nil {
		if driversAPI, ok := driversAPIs[tid]; ok {
			return driversAPI, nil
		}
	} else {
		driversAPIs = make(map[string]*drivers.ConsumerAPI)
	}

	driversAPI, err := InitDriversService(tid)
	if err != nil {
		return nil, err
	}

	driversAPIs[tid] = driversAPI

	return driversAPI, nil
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
