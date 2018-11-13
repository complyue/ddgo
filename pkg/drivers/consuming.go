package drivers

import (
	"sync"

	"github.com/complyue/ddgo/pkg/routes"
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
