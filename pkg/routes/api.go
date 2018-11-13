package routes

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/ddgo/pkg/livecoll"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"sync"
	"time"
)

func NewMonoAPI() *ConsumerAPI {
	return &ConsumerAPI{
		mono: true,
		// all other fields be nil
	}
}

// NewConsumerAPI .
func NewConsumerAPI(tid string) *ConsumerAPI {
	return &ConsumerAPI{
		tid: tid,

		// create reconnection channel
		chReconn: make(chan struct{}),

		// no collection change stream unless subscribed
		wpCCES: nil,

		// initially not connected
		svc: nil,
	}
}

// ConsumerAPI .
type ConsumerAPI struct {
	mono bool // should never be changed after construction

	mu sync.Mutex //

	tid string
	// will be closed on reconnection (with a new chan allocated）
	chReconn chan struct{}

	// collection change event stream for waypoints
	wpCCES *isoevt.EventStream

	svc *hbi.TCPConn
}

// implementation details at consumer endpoint for service consuming over HBI wire
type consumerContext struct {
	hbi.HoContext

	api *ConsumerAPI

	watchingWaypoints bool
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *consumerContext) TypesToExpose() []interface{} {
	return []interface{}{
		(*Waypoint)(nil),
		(*WaypointsSnapshot)(nil),
	}
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
					err = errors.New(fmt.Sprintf("Error connecting to routes service: %+v", e))
				}
			}()
			if api.svc == nil || api.svc.Hosting.Cancelled() || api.svc.Posting.Cancelled() {
				var svc *hbi.TCPConn
				svc, err = svcs.GetService("routes",
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
					// allocate a new reconnection channel, close existing one if present
					chRecon := api.chReconn
					api.chReconn = make(chan struct{})
					if chRecon != nil {
						close(chRecon)
					}
				}
			}
		}()
		if err == nil {
			if api.wpCCES != nil {
				// consumer has subscribed to waypoints collection change event stream,
				// make sure the connected wire has subscribed as well,
				// Epoch event will be fired by service upon each subscription.
				ctx := api.svc.HoCtx().(*consumerContext)
				if !ctx.watchingWaypoints {
					po := api.svc.MustPoToPeer()
					po.Notif(fmt.Sprintf(`
SubscribeWaypoints(%#v)
`, api.tid))
					ctx.watchingWaypoints = true
				}
			}
			return api.svc
		}
		glog.Errorf("Failed connecting routes service, retrying... %+v", err)
		time.Sleep(ReconnectDelay)
	}
}

// get posting endpoint
func (api *ConsumerAPI) conn() (*consumerContext, hbi.Posting) {
	svc := api.EnsureConn()
	return svc.HoCtx().(*consumerContext), svc.MustPoToPeer()
}

func (api *ConsumerAPI) AddWaypoint(tid string, x, y float64) error {
	if api.mono {
		return AddWaypoint(tid, x, y)
	}

	_, po := api.conn()
	return po.Notif(fmt.Sprintf(`
AddWaypoint(%#v,%#v,%#v)
`, tid, x, y))
}

func (api *ConsumerAPI) MoveWaypoint(
	tid string, seq int, id string, x, y float64,
) error {
	if api.mono {
		return MoveWaypoint(tid, seq, id, x, y)
	}

	_, po := api.conn()
	return po.Notif(fmt.Sprintf(`
MoveWaypoint(%#v,%#v,%#v,%#v,%#v)
`, tid, seq, id, x, y))
}

func (api *ConsumerAPI) SubscribeWaypoints(tid string, subr livecoll.Subscriber) {
	if api.wpCCES == nil { // quick check without sync
		func() {
			api.mu.Lock()
			defer api.mu.Unlock()

			if api.wpCCES != nil { // final check after sync'ed
				return
			}

			api.wpCCES = isoevt.NewStream()
		}()
	}
	// now api.wpCCES is guarranteed to not be nil
	// consumer side event stream dispatching for waypoint changes
	livecoll.Dispatch(api.wpCCES, subr, nil)
	// will ensure the wire subscribed as well
	api.EnsureConn()
}

func (ctx *consumerContext) wpCCES() *isoevt.EventStream {
	api := ctx.api
	// api.wpCCES won't change once assigned non-nil, we can trust thread local cache
	cces := api.wpCCES // fast read without sync
	if cces == nil {   // sync'ed read on cache miss
		api.mu.Lock()
		cces = api.wpCCES
		api.mu.Unlock()
	}
	if cces == nil {
		panic("Consumer side wp cces not present on service event ?!")
	}
	return cces
}

func (ctx *consumerContext) WpEpoch(ccn int) {
	cces := ctx.wpCCES()
	cces.Post(livecoll.EpochEvent{ccn})
}

// Create
func (ctx *consumerContext) WpCreated(ccn int) {
	eo, err := ctx.Ho().CoRecvObj()
	if err != nil {
		panic(err)
	}
	wp := eo.(*Waypoint)
	cces := ctx.wpCCES()
	cces.Post(livecoll.CreateEvent{ccn, wp})
}

// Update
func (ctx *consumerContext) WpUpdated(ccn int) {
	eo, err := ctx.Ho().CoRecvObj()
	if err != nil {
		panic(err)
	}
	wp := eo.(*Waypoint)
	cces := ctx.wpCCES()
	cces.Post(livecoll.UpdateEvent{ccn, wp})
}

// Delete
func (ctx *consumerContext) WpDeleted(ccn int) {
	eo, err := ctx.Ho().CoRecvObj()
	if err != nil {
		panic(err)
	}
	wp := eo.(*Waypoint)
	cces := ctx.wpCCES()
	cces.Post(livecoll.DeleteEvent{ccn, wp})
}

func (api *ConsumerAPI) FetchWaypoints(tid string) (ccn int, wpl []Waypoint) {
	_, po := api.conn()
	co, err := po.Co()
	if err != nil {
		panic(err)
	}
	defer co.Close()

	result, err := co.Get(fmt.Sprintf(`
FetchWaypoints(%#v)
`, tid), "&WaypointsSnapshot{}")
	if err != nil {
		panic(err)
	}
	wps := result.(*WaypointsSnapshot)
	if wps.Tid != tid {
		panic(errors.Errorf("Tid mismatch ?! [%s] vs [%s]", wps.Tid, tid))
	}

	ccn, wpl = wps.CCN, wps.Waypoints
	return
}
