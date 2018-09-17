package drivers

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
	return dbc.DB().C("truck")
}

// all trucks of a particular tenant.
// size of this data set is small enough to be fully kept in memory
type TruckList struct {
	Tid string

	Trucks []Truck

	// this is the primary index to locate a truck by tid+seq
	bySeq map[int]*Truck
}

// a single truck
type Truck struct {
	Id bson.ObjectId `json:"_id" bson:"_id"`

	// increase only seq within scope of a tenant
	Seq int `json:"seq"`

	Moving bool    `json:"moving"`
	Label  string  `json:"label"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
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
	fullList  *TruckList
	muListChg sync.Mutex // guard concurrent changes to truck list
)

func ensureLoadedFor(tid string) error {
	// sync is not strictly necessary for load, as worst scenario is to load more than once,
	// while correctness not violated.
	if fullList != nil {
		// already have a full list loaded
		if tid != fullList.Tid {
			// should be coz of malfunctioning of service router, log and deny service
			err := errors.New(fmt.Sprintf(
				"Truck service already stuck to [%s], not serving [%s]!",
				fullList.Tid, tid,
			))
			glog.Error(err)
			return err
		}
		return nil
	}

	// the first time serving a tenant, load full list and stuck to this tid
	loadingList := &TruckList{Tid: tid}
	err := coll().Find(bson.M{"tid": tid}).All(&loadingList.Trucks)
	if err != nil {
		glog.Error(err)
		return err
	}
	optimalSize := 2 * len(loadingList.Trucks)
	if optimalSize < 200 {
		optimalSize = 200
	}
	loadingList.bySeq = make(map[int]*Truck, optimalSize)
	for i := range loadingList.Trucks {
		tk := &loadingList.Trucks[i] // must obtain the pointer this way,
		// not from 2nd loop var, as that'll be a temporary local var
		loadingList.bySeq[tk.Seq] = tk
	}
	fullList = loadingList // only set globally after successfully loaded at all
	return nil
}

func ListTrucks(tid string) (*TruckList, error) {
	if err := ensureLoadedFor(tid); err != nil {
		// err has been logged
		return nil, err
	}
	return fullList, nil
}

func WatchTrucks(
	tid string,
	ackCre func(tk *Truck) bool,
	ackMv func(tid string, seq int, id string, x, y float64) bool,
	ackStop func(tid string, seq int, id string, moving bool) bool,
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
			var knownTail, nextTail *tkCre
			for {
				if knownTail == nil {
					nextTail = tkCreTail
				} else {
					nextTail = knownTail.next
				}
				for nextTail != nil {
					knownTail = nextTail
					if ackCre(&knownTail.tk.Truck) {
						// indicated to stop watching by returning true
						return
					}
					nextTail = knownTail.next
				}
				tkCreated.L.Lock()
				tkCreated.Wait()
				tkCreated.L.Unlock()
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
			var knownTail, nextTail *tkMv
			for {
				if knownTail == nil {
					nextTail = tkMvTail
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
				tkMoved.L.Lock()
				tkMoved.Wait()
				tkMoved.L.Unlock()
			}
		}()
	}

	if ackStop != nil {
		go func() {
			defer func() { // stop watching on watcher func panic, don't crash
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
				}
			}()
			var knownTail, nextTail *tkStop
			for {
				if knownTail == nil {
					nextTail = tkStopTail
				} else {
					nextTail = knownTail.next
				}
				for nextTail != nil {
					knownTail = nextTail
					if ackStop(
						knownTail.tid, knownTail.seq, knownTail.id, knownTail.moving,
					) {
						// indicated to stop watching by returning true
						return
					}
					nextTail = knownTail.next
				}
				tkMoved.L.Lock()
				tkMoved.Wait()
				tkMoved.L.Unlock()
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

// individual in-memory truck objects do not store the tid, tid only
// meaningful for a truck list. however when stored as mongodb documents,
// the tid field needs to present. so here's the struct, with an in-memory
// tk object embedded, to be inlined when marshaled to bson (for mango)
type tkForDb struct {
	Tid   string `bson:"tid"`
	Truck `bson:",inline"`
}

// record for create event
type tkCre struct {
	tk   tkForDb
	next *tkCre
}

// record for move event
type tkMv struct {
	tid  string
	seq  int
	id   string
	x, y float64
	next *tkMv
}

// record for stop event
type tkStop struct {
	tid    string
	seq    int
	id     string
	moving bool
	next   *tkStop
}

var (
	tkCreTail  *tkCre
	tkMvTail   *tkMv
	tkStopTail *tkStop
	// conditions to be waited by watchers for new events
	tkCreated, tkMoved, tkStopped *sync.Cond
)

func init() {
	var (
		tkCreatedLock, tkMovedLock sync.Mutex
	)
	tkCreated = sync.NewCond(&tkCreatedLock)
	tkMoved = sync.NewCond(&tkMovedLock)
}

func AddTruck(tid string, x, y float64) error {
	muListChg.Lock() // is to change tk list, need sync
	muListChg.Unlock()

	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	// sync with event condition for data consistency
	tkCreated.L.Lock()
	defer tkCreated.L.Unlock()

	// prepare the create event record, which contains a truck data object value
	newSeq := 1 + len(fullList.Trucks)      // assign tenant wide unique seq
	newLabel := fmt.Sprintf("#%d#", newSeq) // label with some rules
	newTail := &tkCre{tk: tkForDb{tid, Truck{
		Id:  bson.NewObjectId(),
		Seq: newSeq, Label: newLabel,
		X: x, Y: y,
	}}}
	// write into backing storage, the db
	err := coll().Insert(&newTail.tk)
	if err != nil {
		return err
	}

	// add to in-memory list and index, after successful db insert
	tkl := fullList.Trucks
	insertPos := len(tkl)
	tkl = append(tkl, newTail.tk.Truck)
	fullList.Trucks = tkl
	fullList.bySeq[newTail.tk.Seq] = &tkl[insertPos]

	// publish the create event
	if tkCreTail != nil {
		tkCreTail.next = newTail
	}
	tkCreTail = newTail
	tkCreated.Broadcast()
	return nil
}

func MoveTruck(tid string, seq int, id string, x, y float64) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	tk := fullList.bySeq[seq]
	if tk == nil {
		return errors.New(fmt.Sprintf("Truck seq=%d not exists for tid=%s", seq, tid))
	}
	if id != tk.Id.Hex() {
		return errors.New(fmt.Sprintf("Truck id mismatch [%s] vs [%s]", id, tk.Id.Hex()))
	}

	// sync with event condition for data consistency
	tkMoved.L.Lock()
	defer tkMoved.L.Unlock()

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

	// publish the move event
	newTail := &tkMv{
		tid: tid, seq: tk.Seq, id: tk.Id.Hex(),
		x: x, y: y,
	}
	if tkMvTail != nil {
		tkMvTail.next = newTail
	}
	tkMvTail = newTail
	tkMoved.Broadcast()
	return nil
}

func StopTruck(tid string, seq int, id string, moving bool) error {
	if err := ensureLoadedFor(tid); err != nil {
		return err
	}

	tk := fullList.bySeq[seq]
	if tk == nil {
		return errors.New(fmt.Sprintf("Truck seq=%d not exists for tid=%s", seq, tid))
	}
	if id != tk.Id.Hex() {
		return errors.New(fmt.Sprintf("Truck id mismatch [%s] vs [%s]", id, tk.Id.Hex()))
	}

	// sync with event condition for data consistency
	tkStopped.L.Lock()
	defer tkStopped.L.Unlock()

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

	// publish the stopped event
	newTail := &tkStop{
		tid: tid, seq: tk.Seq, id: tk.Id.Hex(),
		moving: moving,
	}
	if tkStopTail != nil {
		tkStopTail.next = newTail
	}
	tkStopTail = newTail
	tkStopped.Broadcast()
	return nil
}
