package routes

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/dbc"
	"github.com/complyue/ddgo/pkg/isoevt"
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
	bySeq map[int]*Waypoint
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
	loadingList.bySeq = make(map[int]*Waypoint, optimalSize)
	for i := range loadingList.Waypoints {
		wp := &loadingList.Waypoints[i] // must obtain the pointer this way,
		// not from 2nd loop var, as that'll be a temporary local var
		loadingList.bySeq[wp.Seq] = wp
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

var (
	wpCreES, wpMovES *isoevt.EventStream
)

func init() {
	wpCreES = isoevt.NewStream()
	wpMovES = isoevt.NewStream()
}

func WatchWaypoints(
	tid string,
	ackCre func(wp *Waypoint) bool,
	ackMov func(tid string, seq int, id string, x, y float64) bool,
) {
	if err := ensureLoadedFor(tid); err != nil {
		// err has been logged
		return
	}

	if ackCre != nil {
		wpCreES.Watch(func(eo interface{}) bool {
			return ackCre(eo.(*Waypoint))
		})
	}

	if ackMov != nil {
		wpMovES.Watch(func(eo interface{}) bool {
			wp := eo.(*Waypoint)
			return ackMov(tid, wp.Seq, wp.Id.Hex(), wp.X, wp.Y)
		})
	}
}

// individual in-memory waypoint objects do not store the tid, tid only
// meaningful for a waypoint list. however when stored as mongodb documents,
// the tid field needs to present. so here's the struct, with an in-memory
// wp object embedded, to be inlined when marshaled to bson (for mango)
type wpForDb struct {
	Tid      string `bson:"tid"`
	Waypoint `bson:",inline"`
}

func MoveWaypoint(tid string, seq int, id string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	wp := fullList.bySeq[seq]
	if wp == nil {
		return errors.New(fmt.Sprintf("Waypoint seq=%d not exists for tid=%s", seq, tid))
	}
	if id != wp.Id.Hex() {
		return errors.New(fmt.Sprintf("Waypoint id mismatch [%s] vs [%s]", id, wp.Id.Hex()))
	}

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

	wpMovES.Post(wp)

	return nil
}

func AddWaypoint(tid string, x, y float64) error {
	muListChg.Lock() // is to change wp list, need sync
	muListChg.Unlock()

	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	// prepare the create event record, which contains a waypoint data object value
	newSeq := 1 + len(fullList.Waypoints)   // assign tenant wide unique seq
	newLabel := fmt.Sprintf("#%d#", newSeq) // label with some rules
	wp := wpForDb{tid, Waypoint{
		Id:  bson.NewObjectId(),
		Seq: newSeq, Label: newLabel,
		X: x, Y: y,
	}}
	// write into backing storage, the db
	err := coll().Insert(&wp)
	if err != nil {
		return err
	}

	// add to in-memory list and index, after successful db insert
	wpl := fullList.Waypoints
	insertPos := len(wpl)
	wpl = append(wpl, wp.Waypoint)
	waypoint := &wpl[insertPos]
	fullList.Waypoints = wpl
	fullList.bySeq[waypoint.Seq] = waypoint

	// publish the create event
	wpCreES.Post(wp)

	return nil
}
