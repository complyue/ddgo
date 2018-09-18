package drivers

import (
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"math"
	"sync"
	"time"
)

var (
	stuckTid string
	wps      []routes.Waypoint
	wpBySeq  map[int]*routes.Waypoint
)

func DriversKickoff(tid string) error {

	if stuckTid != "" {
		if tid != stuckTid {
			return errors.Errorf("Drivers team already stuck to [%s], not serving [%s]!", stuckTid, tid)
		}
		// kickoff only once
		return nil
	}

	// one time kickoff for the specified tid

	// load & watch waypoints
	routesAPI, err := GetRoutesService("", tid)
	if err != nil {
		return err
	}
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
	WatchTrucks(tid,

		// start a driving immediate when a truck is created,
		// just for demonstration
		func(truck *Truck) (stop bool) {
			go NewDriving(truck).start()
			return
		},

		// the truck pointer points to the authoritative truck data object,
		// which is proc local, it always has the correct location info,
		// so we don't need to watch the move event
		nil,

		// notify the driving goroutine when the truck is told to move or stop
		func(tid string, seq int, id string, moving bool) (stop bool) {
			dr := drivingCourseByTruckSeq[seq]
			dr.toldToMove(moving)
			return
		},
	)

	// list all trucks existing now and start a driving course for each one
	tkl, err := ListTrucks(tid)
	if err != nil {
		// todo cleanup
		return err
	}
	for i := range tkl.Trucks {
		truck := &tkl.Trucks[i]
		go NewDriving(truck).start()
	}

	stuckTid = tid
	return nil
}

// TODO `Driving` should be a relation between a truck and a user, yet persisted

var drivingCourseByTruckSeq = map[int]*Driving{}

func NewDriving(truck *Truck) *Driving {
	dr := &Driving{
		truck:     truck,
		moving:    truck.Moving,
		cndMoving: sync.NewCond(new(sync.Mutex)),
	}
	drivingCourseByTruckSeq[truck.Seq] = dr
	return dr
}

type Driving struct {
	truck     *Truck
	moving    bool
	cndMoving *sync.Cond
}

func (dr *Driving) toldToMove(moving bool) {
	dr.cndMoving.L.Lock()
	dr.moving = moving
	dr.cndMoving.Signal()
	dr.cndMoving.L.Unlock()
}

func (dr *Driving) waitToldBeMoving() (moving bool) {
	dr.cndMoving.L.Lock()
	moving = dr.moving
	dr.cndMoving.L.Unlock()
	for !moving {
		dr.cndMoving.L.Lock()
		dr.cndMoving.Wait()
		moving = dr.moving
		dr.cndMoving.L.Unlock()
	}
	return
}

/* Driving logic
currently simulating a dumb head approaching each waypoint in turn if told to
be moving, or just stay still.
*/
func (dr *Driving) start() {

	const speed = 5
	var (
		wpi = 0
		wp  *routes.Waypoint
	)

	for dr.waitToldBeMoving() {

		if len(wps) < 1 {
			glog.Warning("No waypoint yet.")
			time.Sleep(10 * time.Second)
			continue
		}

		tx, ty := dr.truck.X, dr.truck.Y

		if wpi >= len(wps) {
			wpi = 0
		}
		if wp != &wps[wpi] {
			// find nearest waypoint
			wp = &wps[0]
			dNeareast := math.Sqrt(
				math.Pow(wp.X-tx, 2) + math.Pow(wp.Y-ty, 2),
			)
			for i := 1; i < len(wps); i++ {
				twp := &wps[i]
				d := math.Sqrt(
					math.Pow(twp.X-tx, 2) + math.Pow(twp.Y-ty, 2),
				)
				if d < dNeareast {
					dNeareast = d
					wp = twp
				}
			}
		}

		distance := math.Sqrt(
			math.Pow(wp.X-tx, 2) + math.Pow(wp.Y-ty, 2),
		)
		if distance <= speed {
			// reaching aimed waypoint
			tx, ty = wp.X, wp.Y
			// toward next waypoint
			wpi++
			if wpi >= len(wps) {
				wpi = 0
			}
			wp = &wps[wpi]
		} else {
			// approaching aimed waypoint
			_, tx, ty = 0,
				tx+(wp.X-tx)*speed/distance,
				ty+(wp.Y-ty)*speed/distance
		}

		// `MoveTruck()` is proc local business method, just call directly
		if err := MoveTruck(stuckTid, dr.truck.Seq, dr.truck.Id.Hex(), tx, ty); err != nil {
			glog.Error(errors.Wrap(err, "Truck move failed ?!"))
			return
		}

		// 2 steps per second
		time.Sleep(500 * time.Millisecond)
	}

}
