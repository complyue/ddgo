package routes

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/dbc"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/golang/glog"
	"io"
	"sync"
)

func coll() *mgo.Collection {
	return dbc.DB().C("waypoint")
}

// all waypoints of a particular tenant.
// size of this data set is small enough to be fully kept in memory
type WaypointList struct {
	Tid string

	Waypoints []Waypoint

	// this is the primary index to locate a waypoint by tid+seq
	wpBySeq map[int]*Waypoint
}

// a single waypoint
type Waypoint struct {
	Id bson.ObjectId `json:"_id" bson:"_id"`

	// increase only seq within scope of a tenant
	Seq int `json:"seq"`

	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

func (wp *Waypoint) String() string {
	return fmt.Sprintf("%+v", wp)
}

func (wp *Waypoint) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		io.WriteString(s, fmt.Sprintf("%s", wp.Label))
		if s.Flag('+') {
			io.WriteString(s, fmt.Sprintf("@(%0.1f,%0.1f)", wp.X, wp.Y))
		}
	}
}

var (
	fullList  *WaypointList
	muListChg sync.Mutex // guard concurrent changes to waypoint list
)

func ensureLoadedFor(tid string) error {
	// sync is not strictly necessary for load, as worst scenario is to load more than once,
	// while correctness not violated.
	if fullList != nil {
		// already have a full list loaded
		if tid != fullList.Tid {
			// should be coz of malfunctioning of service router, log and deny service
			err := errors.New(fmt.Sprintf(
				"Waypoint service already stuck to [%s], not serving [%s]!",
				fullList.Tid, tid,
			))
			glog.Error(err)
			return err
		}
		return nil
	}

	// the first time serving a tenant, load full list and stuck to this tid
	loadingList := &WaypointList{Tid: tid}
	err := coll().Find(bson.M{"tid": tid}).All(&loadingList.Waypoints)
	if err != nil {
		glog.Error(err)
		return err
	}
	optimalSize := 2 * len(loadingList.Waypoints)
	if optimalSize < 200 {
		optimalSize = 200
	}
	loadingList.wpBySeq = make(map[int]*Waypoint, optimalSize)
	for i := range loadingList.Waypoints {
		wp := &loadingList.Waypoints[i] // must obtain the pointer this way,
		// not from 2nd loop var, as that'll be a temporary local var
		loadingList.wpBySeq[wp.Seq] = wp
	}
	fullList = loadingList // only set globally after successfully loaded at all
	return nil
}

func ListWaypoints(tid string) (*WaypointList, error) {
	if err := ensureLoadedFor(tid); err != nil {
		// err has been logged
		return nil, err
	}
	return fullList, nil
}

func WatchWaypoints(
	tid string,
	ackCre func(wp *Waypoint) bool,
	ackMv func(tid string, seq int, id string, x, y float64) bool,
) {
	if err := ensureLoadedFor(tid); err != nil {
		// err has been logged
		return
	}

	if ackCre != nil {
		go func() {
			defer func() { // stop watching on watcher func panic, don't crash
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
				}
			}()
			var knownTail, nextTail *wpCre
			for {
				if knownTail == nil {
					nextTail = wpCreTail
				} else {
					nextTail = knownTail.next
				}
				for nextTail != nil {
					knownTail = nextTail
					if ackCre(&knownTail.wp.Waypoint) {
						// indicated to stop watching by returning true
						return
					}
					nextTail = knownTail.next
				}
				wpCreated.L.Lock()
				wpCreated.Wait()
				wpCreated.L.Unlock()
			}
		}()
	}

	if ackMv != nil {
		go func() {
			defer func() { // stop watching on watcher func panic, don't crash
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
				}
			}()
			var knownTail, nextTail *wpMv
			for {
				if knownTail == nil {
					nextTail = wpMvTail
				} else {
					nextTail = knownTail.next
				}
				for nextTail != nil {
					knownTail = nextTail
					if ackMv(
						knownTail.tid, knownTail.seq, knownTail.id,
						knownTail.x, knownTail.y,
					) {
						// indicated to stop watching by returning true
						return
					}
					nextTail = knownTail.next
				}
				wpMoved.L.Lock()
				wpMoved.Wait()
				wpMoved.L.Unlock()
			}
		}()
	}

}

// singly linked lists are used for event records to be consumed, so that:
// given source and each watcher goro has a local tail reference, as source
// keeps rolling forward, watchers will get notified and roll their tails
// forward too. thus an old record object automatically become eligible
// for garbage collection, after all watchers have seen it and rolled their
// tail reference to next. a record object will not be garbage collected
// until all watchers have process it.
// todo the `next` pointer shall be atomic ?

// individual in-memory waypoint objects do not store the tid, tid only
// meaningful for a waypoint list. however when stored as mongodb documents,
// the tid field needs to present. so here's the struct, with an in-memory
// wp object embedded, to be inlined when marshaled to bson (for mango)
type wpForDb struct {
	Tid      string `bson:"tid"`
	Waypoint `bson:",inline"`
}

// record for create event
type wpCre struct {
	wp   wpForDb
	next *wpCre
}

// record for move event
type wpMv struct {
	tid  string
	seq  int
	id   string
	x, y float64
	next *wpMv
}

var (
	wpCreTail *wpCre
	wpMvTail  *wpMv
	// conditions to be waited by watchers for new events
	wpCreated, wpMoved *sync.Cond
)

func init() {
	var (
		wpCreatedLock, wpMovedLock sync.Mutex
	)
	wpCreated = sync.NewCond(&wpCreatedLock)
	wpMoved = sync.NewCond(&wpMovedLock)
}

func MoveWaypoint(tid string, seq int, id string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	wp := fullList.wpBySeq[seq]
	if wp == nil {
		return errors.New(fmt.Sprintf("Waypoint seq=%d not exists for tid=%s", seq, tid))
	}
	if id != wp.Id.Hex() {
		return errors.New(fmt.Sprintf("Waypoint id mismatch [%s] vs [%s]", id, wp.Id.Hex()))
	}

	// sync with event condition for data consistency
	wpMoved.L.Lock()
	defer wpMoved.L.Unlock()

	// update backing storage, the db
	if err := coll().Update(bson.M{
		"tid": tid, "_id": wp.Id,
	}, bson.M{
		"$set": bson.M{"x": x, "y": y},
	}); err != nil {
		return err
	}

	// update in-memory value, after successful db update
	wp.X, wp.Y = x, y

	// publish the move event
	newTail := &wpMv{
		tid: tid, seq: wp.Seq, id: wp.Id.Hex(),
		x: x, y: y,
	}
	if wpMvTail != nil {
		wpMvTail.next = newTail
	}
	wpMvTail = newTail
	wpMoved.Broadcast()
	return nil
}

func AddWaypoint(tid string, x, y float64) error {
	muListChg.Lock() // is to change wp list, need sync
	muListChg.Unlock()

	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	// sync with event condition for data consistency
	wpCreated.L.Lock()
	defer wpCreated.L.Unlock()

	// prepare the create event record, which contains a waypoint data object value
	newSeq := 1 + len(fullList.Waypoints)   // assign tenant wide unique seq
	newLabel := fmt.Sprintf("#%d#", newSeq) // label with some rules
	newTail := &wpCre{wp: wpForDb{tid, Waypoint{
		Id:  bson.NewObjectId(),
		Seq: newSeq, Label: newLabel,
		X: x, Y: y,
	}}}
	// write into backing storage, the db
	err := coll().Insert(&newTail.wp)
	if err != nil {
		return err
	}

	// add to in-memory list and index, after successful db insert
	wpl := fullList.Waypoints
	insertPos := len(wpl)
	wpl = append(wpl, newTail.wp.Waypoint)
	fullList.Waypoints = wpl
	fullList.wpBySeq[newTail.wp.Seq] = &wpl[insertPos]

	// publish the create event
	if wpCreTail != nil {
		wpCreTail.next = newTail
	}
	wpCreTail = newTail
	wpCreated.Broadcast()
	return nil
}
