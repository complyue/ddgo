package routes

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"sync"
	"time"
)

func GetRoutesService(tid string) (*ConsumerAPI, error) {
	api := NewConsumerAPI(tid)
	api.conn()
	return api, nil
}

func NewMonoAPI() *ConsumerAPI {
	return &ConsumerAPI{
		mono: true,
	}
}

// NewConsumerAPI .
func NewConsumerAPI(tid string) *ConsumerAPI {
	return &ConsumerAPI{
		tid: tid,
	}
}

// ConsumerAPI .
type ConsumerAPI struct {
	mono bool // should never be changed after construction

	mu  sync.Mutex
	tid string
	svc *hbi.TCPConn
}

// get posting endpoint
func (api *ConsumerAPI) conn() (*consumerContext, hbi.Posting) {
	svc := api.EnsureConn()
	return svc.HoCtx().(*consumerContext), svc.MustPoToPeer()
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
						}
						ctx.Put("api", api)
						return ctx
					}, // single tunnel, use tid as sticky session id, for tenant isolation
					"", api.tid, true)
				if err == nil {
					api.svc = svc
				}
			}
		}()
		if err == nil {
			return api.svc
		}
		glog.Errorf("Failed connecting routes service, retrying... %+v", err)
		time.Sleep(ReconnectDelay)
	}
}

func (api *ConsumerAPI) ListWaypoints(tid string) (*WaypointList, error) {
	ctx := api.ctx

	if ctx == nil {
		// proc local service consuming
		return ListWaypoints(tid)
	}

	// remote service consuming over HBI wire

	// initiate a conversation
	co, err := ctx.MustPoToPeer().Co()
	if err != nil {
		return nil, err
	}
	defer co.Close()

	// get service method result in rpc style
	wpl, err := co.Get(fmt.Sprintf(`
ListWaypoints(%#v)
`, tid), "&WaypointList{}")
	if err != nil {
		return nil, err
	}

	// return result with type asserted
	return wpl.(*WaypointList), nil
}

func (api *ConsumerAPI) WatchWaypoints(
	tid string,
	ackCre func(wp *Waypoint) bool,
	ackMv func(tid string, seq int, id string, x, y float64) bool,
) {
	ctx := api.ctx

	if ctx == nil {
		// proc local service consuming
		WatchWaypoints(tid, ackCre, ackMv)
		return
	}

	// remote service consuming over HBI wire

	// MustPoToPeer() will RLock, obtain before our RLock, or will deadlock
	p2p := ctx.MustPoToPeer()

	ctx.Lock() // WLock for proper sync
	defer ctx.Unlock()

	if ctx.WatchedTid == "" {
		err := p2p.Notif(fmt.Sprintf(`
WatchWaypoints(%#v)
`, tid))
		if err != nil {
			glog.Errorf("Failed watching waypoint events for tid=%s\n", tid, err)
			// but still add watcher funcs to list by not returning here
		} else {
			ctx.WatchedTid = tid
		}
	} else if tid != ctx.WatchedTid {
		glog.Errorf("Request to watch tid=%s while already be watching %s ?!", tid, ctx.WatchedTid)
		return
	}

	if ackCre != nil {
		ctx.WpCreWatchers = append(ctx.WpCreWatchers, ackCre)
	}
	if ackMv != nil {
		ctx.WpMoveWatchers = append(ctx.WpMoveWatchers, ackMv)
	}
}

func (api *ConsumerAPI) AddWaypoint(tid string, x, y float64) error {
	ctx := api.ctx
	if ctx == nil {
		return AddWaypoint(tid, x, y)
	}

	return ctx.MustPoToPeer().Notif(fmt.Sprintf(`
AddWaypoint(%#v,%#v,%#v)
`, tid, x, y))
}

func (api *ConsumerAPI) MoveWaypoint(
	tid string, seq int, id string, x, y float64,
) error {
	ctx := api.ctx
	if ctx == nil {
		return MoveWaypoint(tid, seq, id, x, y)
	}

	return ctx.MustPoToPeer().Notif(fmt.Sprintf(`
MoveWaypoint(%#v,%#v,%#v,%#v,%#v)
`, tid, seq, id, x, y))
}

// implementation details at consumer endpoint for service consuming over HBI wire
type consumerContext struct {
	hbi.HoContext

	WatchedTid     string
	WpCreWatchers  []func(wp *Waypoint) bool
	WpMoveWatchers []func(tid string, seq int, id string, x, y float64) bool
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *consumerContext) TypesToExpose() []interface{} {
	return []interface{}{
		(*WaypointList)(nil),
		(*Waypoint)(nil),
	}
}

// a consumer side hosting method to relay wp creation notifications
func (ctx *consumerContext) WpCreated() {
	evtObj, err := ctx.Ho().CoRecvObj()
	if err != nil {
		glog.Error(err)
		return
	}
	wp, ok := evtObj.(*Waypoint)
	if !ok {
		err := errors.New(fmt.Sprintf("Sent a %T to WpCreated() ?!", evtObj))
		glog.Error(err)
		return
	}

	ctx.RLock() // RLock for proper sync
	defer ctx.RUnlock()

	cntNils := 0
	for i, n := 0, len(ctx.WpCreWatchers); i < n; i++ {
		ackCre := ctx.WpCreWatchers[i]
		if ackCre == nil {
			// already cleared
			cntNils++
			continue
		}
		func() {
			defer func() {
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
					// clear on error
					ctx.WpCreWatchers[i] = nil
					cntNils++
				}
			}()
			if ackCre(wp) {
				// indicated stop by returning true, clear it
				ctx.WpCreWatchers[i] = nil
				cntNils++
			}
		}()
	}
	if cntNils > len(ctx.WpCreWatchers)/2 {
		// compact the slice to drive nils out, must start a new goro, as this
		// func currently holds a RLock, while `compactWatchers()` will WLock
		go ctx.compactWatchers()
	}
}

// a consumer side hosting method to relay wp move notifications
func (ctx *consumerContext) WpMoved(tid string, seq int, id string, x, y float64) {
	ctx.RLock() // RLock for proper sync
	defer ctx.RUnlock()

	cntNils := 0
	for i, n := 0, len(ctx.WpMoveWatchers); i < n; i++ {
		ackMv := ctx.WpMoveWatchers[i]
		if ackMv == nil {
			// already cleared
			cntNils++
			continue
		}
		func() {
			defer func() {
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
					// clear on error
					ctx.WpMoveWatchers[i] = nil
					cntNils++
				}
			}()
			if ackMv(tid, seq, id, x, y) {
				// indicated stop by returning true, clear it
				ctx.WpMoveWatchers[i] = nil
				cntNils++
			}
		}()
	}
	if cntNils > len(ctx.WpMoveWatchers)/2 {
		// compact the slice to drive nils out, must start a new goro, as this
		// func currently holds a RLock, while `compactWatchers()` will WLock
		go ctx.compactWatchers()
	}
}

// utility method to clear out stopped watcher funcs from watcher list
func (ctx *consumerContext) compactWatchers() {
	ctx.Lock() // WLock for proper sync
	defer ctx.Unlock()

	// todo refactor the compact operation into a utility func,
	// if only comes generics support from Go ...
	var owPos int // overwrite position

	owPos = -1
	for s, ckPos, n := ctx.WpCreWatchers, 0, len(ctx.WpCreWatchers); ckPos < n; ckPos++ {
		if s[ckPos] == nil {
			if owPos < 0 {
				owPos = ckPos
			}
		} else if owPos >= 0 {
			s[owPos], s[ckPos] = s[ckPos], nil
			for owPos++; owPos < ckPos; owPos++ {
				if s[owPos] == nil {
					break
				}
			}
		}
	}
	if owPos >= 0 {
		ctx.WpCreWatchers = ctx.WpCreWatchers[:owPos]
	}

	owPos = -1
	for s, ckPos, n := ctx.WpMoveWatchers, 0, len(ctx.WpMoveWatchers); ckPos < n; ckPos++ {
		if s[ckPos] == nil {
			if owPos < 0 {
				owPos = ckPos
			}
		} else if owPos >= 0 {
			s[owPos], s[ckPos] = s[ckPos], nil
			for owPos++; owPos < ckPos; owPos++ {
				if s[owPos] == nil {
					break
				}
			}
		}
	}
	if owPos >= 0 {
		ctx.WpMoveWatchers = ctx.WpMoveWatchers[:owPos]
	}

}
