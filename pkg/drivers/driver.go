package drivers

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/complyue/ddgo/pkg/livecoll"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
)

var (
	stuckTid string
	mu       sync.Mutex

	wpcLive *wpcCache
)

type wpcCache struct {
	routesAPI *routes.ConsumerAPI      // consuming api to routes service
	ccn       int                      // known change number of the live waypoint collection
	wps       []routes.Waypoint        // local cached waypoint values
	wpBySeq   map[int]*routes.Waypoint // map seq to pointer to waypoints within the `wps` slice
	mu        sync.Mutex               //
}

func (wpc *wpcCache) Epoch(ccn int) (stop bool) {
	// fetch current snapshot of the whole collection
	ccn, wpl := wpc.routesAPI.FetchWaypoints()

	// populate local cache data for the live waypoint collection
	wpc.mu.Lock()
	defer wpc.mu.Unlock()
	wpc.wps = wpl
	wpc.wpBySeq = make(map[int]*routes.Waypoint)
	for i := range wpl {
		wpc.wpBySeq[wpl[i].Seq] = &wpl[i]
	}
	wpc.ccn = ccn

	return
}

// Created
func (wpc *wpcCache) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, wpc.ccn) { // ignore out-dated events
		return
	}
	wp := eo.(*routes.Waypoint)

	wpc.mu.Lock()
	defer wpc.mu.Unlock()
	i := len(wpc.wps)
	wpc.wps = append(wpc.wps, *wp)
	wpc.wpBySeq[wp.Seq] = &wpc.wps[i]
	wpc.ccn = ccn

	return
}

// Updated
func (wpc *wpcCache) MemberUpdated(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, wpc.ccn) { // ignore out-dated events
		return
	}
	wp := eo.(*routes.Waypoint)

	wpc.mu.Lock()
	defer wpc.mu.Unlock()
	*wpc.wpBySeq[wp.Seq] = *wp
	wpc.ccn = ccn

	return
}

// Deleted
func (wpc *wpcCache) MemberDeleted(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, wpc.ccn) { // ignore out-dated events
		return
	}
	wp := eo.(*routes.Waypoint)

	wpc.mu.Lock()
	defer wpc.mu.Unlock()
	// todo remove wp from the slice `wpc.wps`
	delete(wpc.wpBySeq, wp.Seq)
	wpc.ccn = ccn

	return
}

type tkcReact struct {
	// subscribe to trucks live collection, which managed by the local drivers service
}

func (tkc *tkcReact) Epoch(ccn int) (stop bool) {
	// nop
	return
}

// Created
func (tkc *tkcReact) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	tk := eo.(*Truck)

	// start a driving immediate when a truck is created,
	// just for demonstration
	go NewDriving(tk).start()

	return
}

// Updated
func (tkc *tkcReact) MemberUpdated(ccn int, eo livecoll.Member) (stop bool) {
	tk := eo.(*Truck)

	// notify the driving goroutine when the truck is told to move or stop
	dr := drivingCourseByTruckSeq[tk.Seq]
	dr.toldToMove(tk.Moving)

	return
}

// Deleted
func (tkc *tkcReact) MemberDeleted(ccn int, eo livecoll.Member) (stop bool) {
	// todo process delete event
	return
}

func DriversKickoff(tid string) error {

	if stuckTid != "" {
		if tid != stuckTid {
			return errors.Errorf("Drivers team already stuck to [%s], not serving [%s]!", stuckTid, tid)
		}
		// kickoff only once
		return nil
	}

	// one time kickoff for the specified tid

	routesAPI, err := GetRoutesService(tid)
	if err != nil {
		return err
	}

	// create live cache of waypoint collection subscribed from routes service
	wpcLive = &wpcCache{
		routesAPI: routesAPI,
		wpBySeq:   make(map[int]*routes.Waypoint),
	}
	routesAPI.SubscribeWaypoints(wpcLive)

	tkCollection.Subscribe(&tkcReact{})

	// list all trucks existing now and start a driving course for each one
	_, tkl := tkCollection.FetchAll()
	for _, tko := range tkl {
		tk := tko.(*Truck)
		go NewDriving(tk).start()
	}

	stuckTid = tid
	return nil
}

func (api *ConsumerAPI) DriversKickoff(tid string) error {
	if api.mono {
		return DriversKickoff(tid)
	}

	_, po := api.conn()
	return po.Notif(fmt.Sprintf(`
DriversKickoff(%#v)
`, tid))
}

func (ctx *serviceContext) DriversKickoff(tid string) {
	err := DriversKickoff(tid)
	if err != nil {
		panic(err)
	}
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

		wps := wpcLive.wps

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
