package routes

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/dbc"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/golang/glog"
	"sync"
)

func coll() *mgo.Collection {
	return dbc.DB().C("waypoint")
}

type Waypoint map[string]interface{}

var (
	stickyTid string
	lastSeq   = -1
)

/*
   Once stuck to a tenant (by tid), this proc assumes the sole authority to
   create new waypoints for this tenant, so we can cache last seq assigned,
   and increase 1 per successful creation of a waypoint for this tenant.

*/
func NextWpSeq(tid string) int {
	if lastSeq < 0 || tid != stickyTid {
		lastSeq = 0
		var seqResult []struct{ Seq int }
		err := coll().Find(bson.M{"tid": tid}).Select(bson.M{"seq": 1}).Sort("-seq").Limit(1).All(&seqResult)
		if err != nil {
			glog.Error(errors.RichError(err))
		} else if len(seqResult) > 0 {
			lastSeq = seqResult[0].Seq
		}
	}
	return lastSeq + 1
}

func ListWaypoints(tid string) ([]Waypoint, error) {
	var wps []Waypoint
	err := coll().Find(bson.M{"tid": tid}).All(&wps)
	return wps, err
}

func WatchWaypoints(
	tid string,
	ackCre func(wp Waypoint) bool,
	ackMv func(id string, x, y float64) bool,
) {
	if stickyTid == "" {
		NextWpSeq(tid)
	}
	if stickyTid != "" && tid != stickyTid {
		glog.Warningf("Not watching for tid=[%v] as this proc already stuck to [%v]",
			tid, stickyTid)
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
					if ackCre(knownTail.wp) {
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
					if ackMv(knownTail.id, knownTail.x, knownTail.y) {
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

type wpCre struct {
	wp   Waypoint
	next *wpCre
}

type wpMv struct {
	id   string
	x, y float64
	next *wpMv
}

var (
	// use singly linked lists for event data to be consumed,
	// every watcher goro has its local tail reference, so
	// a data object automatically become subject to garbage collection,
	// after all watchers have seen it and updated their tail reference
	wpCreTail *wpCre
	wpMvTail  *wpMv
	// conditions to hold watchers when no event happens
	wpCreated, wpMoved *sync.Cond
)

func init() {
	var (
		wpCreatedLock, wpMovedLock sync.Mutex
	)
	wpCreated = sync.NewCond(&wpCreatedLock)
	wpMoved = sync.NewCond(&wpMovedLock)
}

func MoveWaypoint(tid string, id string, x, y float64) (err error) {
	if err = coll().Update(bson.M{
		"tid": tid, "_id": bson.ObjectIdHex(id),
	}, bson.M{
		"$set": bson.M{"x": x, "y": y},
	}); err != nil {
		return
	}

	wpMoved.L.Lock()
	defer wpMoved.L.Unlock()
	newTail := &wpMv{id, x, y, nil}
	if wpMvTail != nil {
		wpMvTail.next = newTail
	}
	wpMvTail = newTail
	wpMoved.Broadcast()
	return
}

func AddWaypoint(tid string, x, y float64) (err error) {
	newSeq := NextWpSeq(tid)
	wp := Waypoint{
		"_id": bson.NewObjectId(),
		"tid": tid, "seq": newSeq,
		"x": x, "y": y,
		"label": fmt.Sprintf("#%d#", newSeq),
	}
	err = coll().Insert(wp)
	if err != nil {
		return
	}
	lastSeq = newSeq

	wpCreated.L.Lock()
	defer wpCreated.L.Unlock()
	newTail := &wpCre{wp, nil}
	if wpCreTail != nil {
		wpCreTail.next = newTail
	}
	wpCreTail = newTail
	wpCreated.Broadcast()
	return
}
