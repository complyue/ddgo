package drivers

import (
	"fmt"
	"sync"
	"time"

	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/ddgo/pkg/livecoll"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
)

func NewMonoAPI(tid string) *ConsumerAPI {
	return &ConsumerAPI{
		mono: true,
		tid:  tid,
		// all other fields be nil
	}
}

// NewConsumerAPI .
func NewConsumerAPI(tid string) *ConsumerAPI {
	return &ConsumerAPI{
		tid: tid,

		// no collection change stream unless subscribed
		tkCCES: nil,

		// initially not connected
		svc: nil,
	}
}

// ConsumerAPI .
type ConsumerAPI struct {
	mono bool // should never be changed after construction

	mu sync.Mutex //

	tid string

	// collection change event stream for Trucks
	tkCCES *isoevt.EventStream
	tkCCN  int // last known ccn of truck collection

	svc *hbi.TCPConn
}

// implementation details at consumer endpoint for service consuming over HBI wire
type consumerContext struct {
	hbi.HoContext

	api *ConsumerAPI

	watchingTrucks bool
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *consumerContext) TypesToExpose() []interface{} {
	return []interface{}{
		(*Truck)(nil),
		(*TrucksSnapshot)(nil),
	}
}

// Tid getter
func (api *ConsumerAPI) Tid() string {
	return api.tid
}

func (api *ConsumerAPI) EnsureAlive() {
	if api.mono {
		return
	}
	api.EnsureConn()
}

const ReconnectDelay = 3 * time.Second

// ensure connected to a service endpoint via hbi wire
func (api *ConsumerAPI) EnsureConn() *hbi.TCPConn {
	if api.mono {
		panic(errors.New("This api is running in monolith mode."))
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	var err error
	for {
		func() {
			defer func() {
				if e := recover(); e != nil {
					err = errors.New(fmt.Sprintf("Error connecting to drivers service: %+v", e))
				}
			}()
			if api.svc == nil || api.svc.Hosting.Cancelled() || api.svc.Posting.Cancelled() {
				var svc *hbi.TCPConn
				svc, err = svcs.GetService("drivers",
					func() hbi.HoContext {
						ctx := &consumerContext{
							HoContext: hbi.NewHoContext(),
							api:       api,
						}
						return ctx
					}, // single tunnel, use tid as sticky session id, for tenant isolation
					"", api.tid, true)
				if err == nil {
					api.svc = svc
				}
			}
		}()
		if err == nil {
			if api.tkCCES != nil {
				// consumer has subscribed to Trucks collection change event stream,
				// make sure the connected wire has subscribed as well,
				// Epoch event will be fired by service upon each subscription.
				ctx := api.svc.HoCtx().(*consumerContext)
				if !ctx.watchingTrucks {
					po := api.svc.MustPoToPeer()
					po.Notif(fmt.Sprintf(`
SubscribeTrucks(%#v)
`, api.tid))
					ctx.watchingTrucks = true
				}
			}
			return api.svc
		}
		glog.Errorf("Failed connecting drivers service, retrying... %+v", err)
		time.Sleep(ReconnectDelay)
	}
}

// get posting endpoint
func (api *ConsumerAPI) conn() (*consumerContext, hbi.Posting) {
	svc := api.EnsureConn()
	return svc.HoCtx().(*consumerContext), svc.MustPoToPeer()
}

func (api *ConsumerAPI) AddTruck(tid string, x, y float64) error {
	if api.mono {
		return AddTruck(tid, x, y)
	}

	_, po := api.conn()
	return po.Notif(fmt.Sprintf(`
AddTruck(%#v,%#v,%#v)
`, tid, x, y))
}

func (api *ConsumerAPI) MoveTruck(
	tid string, seq int, id string, x, y float64,
) error {
	if api.mono {
		return MoveTruck(tid, seq, id, x, y)
	}

	glog.V(1).Infof(" * Requesting truck %v to move to (%v,%v) ...", seq, x, y)
	_, po := api.conn()
	err := po.Notif(fmt.Sprintf(`
	MoveTruck(%#v,%#v,%#v,%#v,%#v)
	`, tid, seq, id, x, y))
	glog.V(1).Infof(" * Requested truck %v to move to (%v,%v).", seq, x, y)
	return err
}

func (api *ConsumerAPI) StopTruck(
	tid string, seq int, id string, moving bool,
) error {
	if api.mono {
		return StopTruck(tid, seq, id, moving)
	}

	glog.V(1).Infof(" * Requested truck %v to be moving=%v.", seq, moving)
	_, po := api.conn()
	err := po.Notif(fmt.Sprintf(`
StopTruck(%#v,%#v,%#v,%#v)
`, tid, seq, id, moving))
	glog.V(1).Infof(" * Requested truck %v to be moving=%v.", seq, moving)
	return err
}

func (api *ConsumerAPI) FetchTrucks() (ccn int, tkl []Truck) {
	if api.mono {
		tks := FetchTrucks(api.tid)

		ccn, tkl = tks.CCN, tks.Trucks
		return
	}

	_, po := api.conn()
	co, err := po.Co()
	if err != nil {
		panic(err)
	}
	defer co.Close()

	result, err := co.Get(fmt.Sprintf(`
FetchTrucks(%#v)
`, api.tid), "&TrucksSnapshot{}")
	if err != nil {
		panic(err)
	}
	tks := result.(*TrucksSnapshot)

	ccn, tkl = tks.CCN, tks.Trucks
	return
}

func (api *ConsumerAPI) SubscribeTrucks(subr livecoll.Subscriber) {
	if api.mono {
		ensureLoadedFor(api.tid)
		tkCollection.Subscribe(subr)
		return
	}

	// ensure the wire is connected
	api.EnsureConn()

	if api.tkCCES == nil { // quick check without sync
		func() {
			api.mu.Lock()
			defer api.mu.Unlock()

			if api.tkCCES != nil { // final check after sync'ed
				return
			}

			api.tkCCES = isoevt.NewStream()
		}()
	}
	// now api.tkCCES is guarranteed to not be nil
	// consumer side event stream dispatching for Truck changes
	livecoll.Dispatch(api.tkCCES, subr, func() bool {
		// fire Epoch event upon watching started
		subr.Epoch(api.tkCCN)
		return false
	})
}

func (ctx *consumerContext) tkCCES() *isoevt.EventStream {
	api := ctx.api
	// api.tkCCES won't change once assigned non-nil, we can trust thread local cache
	cces := api.tkCCES // fast read without sync
	if cces == nil {   // sync'ed read on cache miss
		api.mu.Lock()
		cces = api.tkCCES
		api.mu.Unlock()
	}
	if cces == nil {
		panic("Consumer side tk cces not present on service event ?!")
	}
	return cces
}

func (ctx *consumerContext) TkEpoch(ccn int) {
	cces := ctx.tkCCES()
	cces.Post(livecoll.EpochEvent{ccn})
}

// Create
func (ctx *consumerContext) TkCreated(ccn int) {
	eo, err := ctx.Ho().CoRecvObj()
	if err != nil {
		panic(err)
	}
	tk := eo.(*Truck)
	cces := ctx.tkCCES()
	cces.Post(livecoll.CreatedEvent{ccn, tk})
}

// Update
func (ctx *consumerContext) TkUpdated(ccn int) {
	eo, err := ctx.Ho().CoRecvObj()
	if err != nil {
		panic(err)
	}
	tk := eo.(*Truck)
	cces := ctx.tkCCES()
	cces.Post(livecoll.UpdatedEvent{ccn, tk})
}

// Delete
func (ctx *consumerContext) TkDeleted(ccn int) {
	eo, err := ctx.Ho().CoRecvObj()
	if err != nil {
		panic(err)
	}
	tk := eo.(*Truck)
	cces := ctx.tkCCES()
	cces.Post(livecoll.DeletedEvent{ccn, tk})
}
