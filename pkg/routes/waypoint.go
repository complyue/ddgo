package routes

import (
	"fmt"
	"io"

	"github.com/complyue/ddgo/pkg/dbc"
	"github.com/complyue/ddgo/pkg/livecoll"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/golang/glog"
)

func coll() *mgo.Collection {
	return dbc.DB().C("waypoint")
}

// in-memory storage of all waypoints of a particular tenant.
// this data set should have a small footprint, small enough to be fully kept in memory
type WaypointCollection struct {
	livecoll.HouseKeeper

	Tid string
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

func (wp *Waypoint) GetID() interface{} {
	return wp.Id
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
	wpCollection *WaypointCollection
)

func ensureLoadedFor(tid string) error {
	// sync is not strictly necessary for load, as worst scenario is to load more than once,
	// while correctness not violated.
	if wpCollection != nil {
		// already have a full list loaded
		if tid != wpCollection.Tid {
			// should be coz of malfunctioning of service router, log and deny service
			err := errors.New(fmt.Sprintf(
				"Waypoint service already stuck to [%s], not serving [%s]!",
				wpCollection.Tid, tid,
			))
			glog.Error(err)
			return err
		}
		return nil
	}

	// the first time serving a tenant, load full list and stuck to this tid
	var loadingList []Waypoint
	err := coll().Find(bson.M{"tid": tid}).All(&loadingList)
	if err != nil {
		glog.Error(err)
		return err
	}
	optimalSize := 2 * len(loadingList)
	if optimalSize < 200 {
		optimalSize = 200
	}
	var hk livecoll.HouseKeeper
	if wpCollection != nil {
		// inherite subscribers by reusing the housekeeper, if already loaded & subscribed
		hk = wpCollection.HouseKeeper
	} else {
		hk = livecoll.NewHouseKeeper()
	}
	loadingColl := &WaypointCollection{
		HouseKeeper: hk,
		Tid:         tid,
		bySeq:       make(map[int]*Waypoint, optimalSize),
	}
	memberList := make([]livecoll.Member, len(loadingList))
	for i, wpo := range loadingList {
		// the loop var is a fixed value variable of Waypoint struct,
		// make a local copy and take pointer for collection storage.
		wpCopy := wpo
		memberList[i] = &wpCopy
		loadingColl.bySeq[wpo.Seq] = &wpCopy
	}
	hk.Load(memberList)
	wpCollection = loadingColl // only set globally after successfully loaded at all
	return nil
}

// the snapshot of all waypoints of a specific tenant
type WaypointsSnapshot struct {
	Tid       string
	CCN       int
	Waypoints []Waypoint
}

func FetchWaypoints(tid string) *WaypointsSnapshot {
	if err := ensureLoadedFor(tid); err != nil {
		// err has been logged
		panic(err)
	}
	// todo make FetchAll return values instead of pointers, to avoid dirty value reads
	ccn, wps := wpCollection.FetchAll()
	snap := &WaypointsSnapshot{
		Tid:       tid,
		CCN:       ccn,
		Waypoints: make([]Waypoint, len(wps)),
	}
	for i, wp := range wps {
		snap.Waypoints[i] = *(wp.(*Waypoint))
	}
	return snap
}

// this service method has rpc style, with err-out converted to panic,
// which will induce forceful disconnection
func (ctx *serviceContext) FetchWaypoints(tid string) *WaypointsSnapshot {
	return FetchWaypoints(tid)
}

type wpDelegate struct {
	ctx *serviceContext
}

func (ctx *serviceContext) SubscribeWaypoints(tid string) {
	if err := ensureLoadedFor(tid); err != nil {
		panic(err)
	}

	dele := wpDelegate{ctx}
	wpCollection.Subscribe(dele)
}

func (dele wpDelegate) Epoch(ccn int) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	po := ctx.MustPoToPeer()
	po.Notif(fmt.Sprintf(`
WpEpoch(%#v)
`, ccn))
	return
}

// Created
func (dele wpDelegate) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	wp := eo.(*Waypoint)
	po := ctx.MustPoToPeer()
	po.NotifBSON(fmt.Sprintf(`
WpCreated(%#v)
`, ccn), wp, "&Waypoint{}")
	return
}

// Updated
func (dele wpDelegate) MemberUpdated(ccn int, eo livecoll.Member) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	wp := eo.(*Waypoint)
	po := ctx.MustPoToPeer()
	po.NotifBSON(fmt.Sprintf(`
WpUpdated(%#v)
`, ccn), wp, "&Waypoint{}")
	return
}

// Deleted
func (dele wpDelegate) MemberDeleted(ccn int, id interface{}) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	po := ctx.MustPoToPeer()
	po.NotifBSON(fmt.Sprintf(`
WpDeleted(%#v,%#v)
`, ccn), id, "&Waypoint{}")
	return
}

// individual in-memory waypoint objects do not store the tid, tid only
// meaningful for a waypoint collection. however when stored as mongodb documents,
// the tid field needs to present. so here's the struct, with an in-memory
// wp object embedded, to be inlined when marshaled to bson (for mango)
type wpForDb struct {
	Tid      string `bson:"tid"`
	Waypoint `bson:",inline"`
}

func AddWaypoint(tid string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	newSeq := 1 + len(wpCollection.bySeq)   // assign tenant wide unique seq
	newLabel := fmt.Sprintf("#%d#", newSeq) // label with some rules
	waypoint := wpForDb{tid, Waypoint{
		Id:  bson.NewObjectId(),
		Seq: newSeq, Label: newLabel,
		X: x, Y: y,
	}}
	// write into backing storage, the db
	err := coll().Insert(&waypoint)
	if err != nil {
		return err
	}

	// add to in-memory collection and index, after successful db insert
	wp := &waypoint.Waypoint
	wpCollection.bySeq[waypoint.Seq] = wp
	wpCollection.Created(wp)

	return nil
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) AddWaypoint(tid string, x, y float64) error {
	return AddWaypoint(tid, x, y)
}

func MoveWaypoint(tid string, seq int, id string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	mwp, ok := wpCollection.Read(bson.ObjectIdHex(id))
	if !ok || mwp == nil {
		return errors.New(fmt.Sprintf("Waypoint seq=[%v], id=[%s] not exists for tid=%s", seq, id, tid))
	}
	wp := mwp.(*Waypoint)
	if wp.Seq != seq {
		return errors.New(fmt.Sprintf("Waypoint id=[%s], seq mismatch [%v] vs [%v]", id, seq, wp.Seq))
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

	wpCollection.Updated(wp)

	return nil
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) MoveWaypoint(
	tid string, seq int, id string, x, y float64,
) error {
	return MoveWaypoint(tid, seq, id, x, y)
}
