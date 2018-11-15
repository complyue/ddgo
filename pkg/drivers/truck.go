package drivers

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
	return dbc.DB().C("truck")
}

// in-memory storage of all trucks of a particular tenant.
// this data set should have a small footprint, small enough to be fully kept in memory
type TruckCollection struct {
	livecoll.HouseKeeper

	Tid string
	// this is the primary index to locate a truck by tid+seq
	bySeq map[int]*Truck
}

// a single truck
type Truck struct {
	Id bson.ObjectId `json:"_id" bson:"_id"`

	// increase only seq within scope of a tenant
	Seq int `json:"seq"`

	Label  string  `json:"label"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Moving bool    `json:"moving"`
}

func (tk *Truck) GetID() interface{} {
	return tk.Id
}

func (tk *Truck) String() string {
	return fmt.Sprintf("%+v", tk)
}

func (tk *Truck) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		io.WriteString(s, fmt.Sprintf("%s", tk.Label))
		if s.Flag('+') {
			io.WriteString(s, fmt.Sprintf("@(%0.1f,%0.1f)", tk.X, tk.Y))
			if tk.Moving {
				io.WriteString(s, "*")
			}
		}
	}
}

var (
	tkCollection *TruckCollection
)

func ensureLoadedFor(tid string) error {
	// sync is not strictly necessary for load, as worst scenario is to load more than once,
	// while correctness not violated.
	if tkCollection != nil {
		// already have a full list loaded
		if tid != tkCollection.Tid {
			// should be coz of malfunctioning of service router, log and deny service
			err := errors.New(fmt.Sprintf(
				"Truck service already stuck to [%s], not serving [%s]!",
				tkCollection.Tid, tid,
			))
			glog.Error(err)
			return err
		}
		return nil
	}

	// the first time serving a tenant, load full list and stuck to this tid
	var loadingList []Truck
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
	if tkCollection != nil {
		// inherite subscribers by reusing the housekeeper, if already loaded & subscribed
		hk = tkCollection.HouseKeeper
	} else {
		hk = livecoll.NewHouseKeeper()
	}
	loadingColl := &TruckCollection{
		HouseKeeper: hk,
		Tid:         tid,
		bySeq:       make(map[int]*Truck, optimalSize),
	}
	memberList := make([]livecoll.Member, len(loadingList))
	for i, tko := range loadingList {
		// the loop var is a fixed value variable of Waypoint struct,
		// make a local copy and take pointer for collection storage.
		tkCopy := tko
		memberList[i] = &tkCopy
		loadingColl.bySeq[tko.Seq] = &tkCopy
	}
	hk.Load(memberList)
	tkCollection = loadingColl // only set globally after successfully loaded at all
	return nil
}

// the snapshot of all Trucks of a specific tenant
type TrucksSnapshot struct {
	Tid    string
	CCN    int
	Trucks []Truck
}

func FetchTrucks(tid string) *TrucksSnapshot {
	if err := ensureLoadedFor(tid); err != nil {
		// err has been logged
		panic(err)
	}
	// todo make FetchAll return values instead of pointers, to avoid dirty value reads
	ccn, tks := tkCollection.FetchAll()
	snap := &TrucksSnapshot{
		Tid:    tid,
		CCN:    ccn,
		Trucks: make([]Truck, len(tks)),
	}
	for i, tk := range tks {
		snap.Trucks[i] = *(tk.(*Truck))
	}
	return snap
}

// this service method has rpc style, with err-out converted to panic,
// which will induce forceful disconnection
func (ctx *serviceContext) FetchTrucks(tid string) *TrucksSnapshot {
	return FetchTrucks(tid)
}

type tkDelegate struct {
	ctx *serviceContext
}

func (ctx *serviceContext) SubscribeTrucks(tid string) {
	if err := ensureLoadedFor(tid); err != nil {
		panic(err)
	}

	dele := tkDelegate{ctx}
	tkCollection.Subscribe(dele)
}

func (dele tkDelegate) Epoch(ccn int) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	po := ctx.MustPoToPeer()
	po.Notif(fmt.Sprintf(`
TkEpoch(%#v)
`, ccn))
	return
}

// Created
func (dele tkDelegate) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	tk := eo.(*Truck)
	po := ctx.MustPoToPeer()
	po.NotifBSON(fmt.Sprintf(`
TkCreated(%#v)
`, ccn), tk, "&Truck{}")
	return
}

// Updated
func (dele tkDelegate) MemberUpdated(ccn int, eo livecoll.Member) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	tk := eo.(*Truck)
	po := ctx.MustPoToPeer()
	if err := po.NotifBSON(fmt.Sprintf(`
TkUpdated(%#v)
`, ccn), tk, "&Truck{}"); err != nil {
		stop = true
		return
	}
	return
}

// Deleted
func (dele tkDelegate) MemberDeleted(ccn int, id interface{}) (stop bool) {
	ctx := dele.ctx
	if ctx.Cancelled() {
		stop = true
		return
	}
	po := ctx.MustPoToPeer()
	po.Notif(fmt.Sprintf(`
TkDeleted(%#v,%#v)
`, ccn, id))
	return
}

// individual in-memory Truck objects do not store the tid, tid only
// meaningful for a Truck collection. however when stored as mongodb documents,
// the tid field needs to present. so here's the struct, with an in-memory
// tk object embedded, to be inlined when marshaled to bson (for mango)
type tkForDb struct {
	Tid   string `bson:"tid"`
	Truck `bson:",inline"`
}

func AddTruck(tid string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	newSeq := 1 + len(tkCollection.bySeq)   // assign tenant wide unique seq
	newLabel := fmt.Sprintf("#%d#", newSeq) // label with some rules
	Truck := tkForDb{tid, Truck{
		Id:  bson.NewObjectId(),
		Seq: newSeq, Label: newLabel,
		X: x, Y: y,
		Moving: false,
	}}
	// write into backing storage, the db
	err := coll().Insert(&Truck)
	if err != nil {
		return err
	}

	// add to in-memory collection and index, after successful db insert
	tk := &Truck.Truck
	tkCollection.bySeq[Truck.Seq] = tk
	tkCollection.Created(tk)

	return nil
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) AddTruck(tid string, x, y float64) error {
	return AddTruck(tid, x, y)
}

func MoveTruck(tid string, seq int, id string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	mtk, ok := tkCollection.Read(bson.ObjectIdHex(id))
	if !ok || mtk == nil {
		return errors.New(fmt.Sprintf("Truck seq=[%v], id=[%s] not exists for tid=%s", seq, id, tid))
	}
	tk := mtk.(*Truck)
	if tk.Seq != seq {
		return errors.New(fmt.Sprintf("Truck id=[%s], seq mismatch [%v] vs [%v]", id, seq, tk.Seq))
	}

	// update backing storage, the db
	if err := coll().Update(bson.M{
		"tid": tid, "_id": tk.Id,
	}, bson.M{
		"$set": bson.M{"x": x, "y": y},
	}); err != nil {
		return err
	}

	// update in-memory value, after successful db update
	tk.X, tk.Y = x, y

	tkCollection.Updated(tk)

	return nil
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) MoveTruck(
	tid string, seq int, id string, x, y float64,
) error {
	glog.V(1).Infof(" * Moving truck %v to (%v,%v) by service ...", seq, x, y)
	err := MoveTruck(tid, seq, id, x, y)
	glog.V(1).Infof(" * Moved truck %v to (%v,%v) by service, err=%+v.", seq, x, y, err)
	return err
}

func StopTruck(tid string, seq int, id string, moving bool) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	mtk, ok := tkCollection.Read(bson.ObjectIdHex(id))
	if !ok || mtk == nil {
		return errors.New(fmt.Sprintf("Truck seq=[%v], id=[%s] not exists for tid=%s", seq, id, tid))
	}
	tk := mtk.(*Truck)
	if tk.Seq != seq {
		return errors.New(fmt.Sprintf("Truck id=[%s], seq mismatch [%v] vs [%v]", id, seq, tk.Seq))
	}

	// update backing storage, the db
	if err := coll().Update(bson.M{
		"tid": tid, "_id": tk.Id,
	}, bson.M{
		"$set": bson.M{"moving": moving},
	}); err != nil {
		return err
	}

	// update in-memory value, after successful db update
	tk.Moving = moving

	tkCollection.Updated(tk)

	return nil
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) StopTruck(
	tid string, seq int, id string, moving bool,
) error {
	glog.V(1).Infof(" * Making truck %v to moving=%v by service ...", seq, moving)
	err := StopTruck(tid, seq, id, moving)
	glog.V(1).Infof(" * Made truck %v to moving=%v by service.", seq, moving)
	return err
}
