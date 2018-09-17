package drivers

import (
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/hbigo/pkg/errors"
)

var (
	stuckTid string
	wps      []routes.Waypoint
	wpBySeq  map[int]*routes.Waypoint
)

func DriversKickoff(tid string) error {

	if stuckTid == "" {
		routesAPI, err := GetRoutesService("", tid)
		if err != nil {
			return err
		}

		// load & watch waypoints
		wpl, err := routesAPI.ListWaypoints(tid)
		if err != nil {
			return err
		}
		wps = make([]routes.Waypoint, len(wpl.Waypoints), 2*len(wpl.Waypoints))
		wpBySeq = make(map[int]*routes.Waypoint, 2*len(wpl.Waypoints))
		for i := range wpl.Waypoints {
			wps[i] = wpl.Waypoints[i]
			wpBySeq[wps[i].Seq] = &wps[i]
		}
		routesAPI.WatchWaypoints(tid, func(wp *routes.Waypoint) (stop bool) {
			wps = append(wps, *wp)
			wpBySeq[wp.Seq] = &wps[len(wps)-1]
			return
		}, func(tid string, seq int, id string, x, y float64) (stop bool) {
			wp := wpBySeq[seq]
			wp.X, wp.Y = x, y
			return
		})

		// manage drivers
		WatchTrucks(tid, func(wp *Truck) (stop bool) {

			return
		}, func(tid string, seq int, id string, x, y float64) (stop bool) {

			return
		}, func(tid string, seq int, id string, moving bool) (stop bool) {

			return
		})

		stuckTid = tid
		return nil
	} else if tid != stuckTid {
		return errors.Errorf("Drivers team already stuck to [%s], not serving [%s]!", stuckTid, tid)
	}

	return nil
}
