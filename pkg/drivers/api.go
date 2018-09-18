package drivers

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
)

func GetDriversService(tid string) (*ConsumerAPI, error) {
	if svc, err := svcs.GetService("drivers", func() hbi.HoContext {
		api := NewConsumerAPI()
		ctx := api.GetHoCtx()
		ctx.Put("api", api)
		return ctx
	}, // single tunnel, use tid as sticky session id, for tenant isolation
		"", tid, true); err != nil {
		return nil, err
	} else {
		return svc.Hosting.HoCtx().Get("api").(*ConsumerAPI), nil
	}
}

func NewConsumerAPI() *ConsumerAPI {
	return &ConsumerAPI{}
}

type ConsumerAPI struct {
	ctx *consumerContext
}

// once invoked, the returned ctx must be used to establish a HBI connection
// to a remote service.
func (api *ConsumerAPI) GetHoCtx() hbi.HoContext {
	if api.ctx == nil {
		api.ctx = &consumerContext{
			HoContext: hbi.NewHoContext(),
		}
	}
	return api.ctx
}

func (api *ConsumerAPI) ListTrucks(tid string) (*TruckList, error) {
	ctx := api.ctx

	if ctx == nil {
		// proc local service consuming
		return ListTrucks(tid)
	}

	// remote service consuming over HBI wire

	// initiate a conversation
	co, err := ctx.PoToPeer().Co()
	if err != nil {
		return nil, err
	}
	defer co.Close()

	// get service method result in rpc style
	wpl, err := co.Get(fmt.Sprintf(`
ListTrucks(%#v)
`, tid), "&TruckList{}")
	if err != nil {
		return nil, err
	}

	// return result with type asserted
	return wpl.(*TruckList), nil
}

func (api *ConsumerAPI) WatchTrucks(
	tid string,
	ackCre func(wp *Truck) bool,
	ackMv func(tid string, seq int, id string, x, y float64) bool,
	ackStop func(tid string, seq int, id string, moving bool) bool,
) {
	ctx := api.ctx

	if ctx == nil {
		// proc local service consuming
		WatchTrucks(tid, ackCre, ackMv, ackStop)
		return
	}

	// remote service consuming over HBI wire

	// PoToPeer() will RLock, obtain before our RLock, or will deadlock
	p2p := ctx.PoToPeer()

	ctx.Lock() // WLock for proper sync
	defer ctx.Unlock()

	if ctx.WatchedTid == "" {
		err := p2p.Notif(fmt.Sprintf(`
WatchTrucks(%#v)
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
		ctx.TkCreWatchers = append(ctx.TkCreWatchers, ackCre)
	}
	if ackMv != nil {
		ctx.TkMoveWatchers = append(ctx.TkMoveWatchers, ackMv)
	}
	if ackStop != nil {
		ctx.TkStopWatchers = append(ctx.TkStopWatchers, ackStop)
	}
}

func (api *ConsumerAPI) AddTruck(tid string, x, y float64) error {
	ctx := api.ctx
	if ctx == nil {
		return AddTruck(tid, x, y)
	}

	return ctx.PoToPeer().Notif(fmt.Sprintf(`
AddTruck(%#v,%#v,%#v)
`, tid, x, y))
}

func (api *ConsumerAPI) MoveTruck(
	tid string, seq int, id string, x, y float64,
) error {
	ctx := api.ctx
	if ctx == nil {
		return MoveTruck(tid, seq, id, x, y)
	}

	return ctx.PoToPeer().Notif(fmt.Sprintf(`
MoveTruck(%#v,%#v,%#v,%#v,%#v)
`, tid, seq, id, x, y))
}

func (api *ConsumerAPI) StopTruck(
	tid string, seq int, id string, moving bool,
) error {
	ctx := api.ctx
	if ctx == nil {
		return StopTruck(tid, seq, id, moving)
	}

	return ctx.PoToPeer().Notif(fmt.Sprintf(`
StopTruck(%#v,%#v,%#v,%#v)
`, tid, seq, id, moving))
}

func (api *ConsumerAPI) DriversKickoff(tid string) error {
	ctx := api.ctx

	if ctx == nil {
		// proc local service consuming
		return DriversKickoff(tid)
	}

	// remote service consuming over HBI wire

	// get service method result in rpc style
	err := ctx.PoToPeer().Notif(fmt.Sprintf(`
DriversKickoff(%#v)
`, tid))
	if err != nil {
		return err
	}

	return nil
}

// implementation details at consumer endpoint for service consuming over HBI wire
type consumerContext struct {
	hbi.HoContext

	WatchedTid     string
	TkCreWatchers  []func(wp *Truck) bool
	TkMoveWatchers []func(tid string, seq int, id string, x, y float64) bool
	TkStopWatchers []func(tid string, seq int, id string, moving bool) bool
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *consumerContext) TypesToExpose() []interface{} {
	return []interface{}{
		(*TruckList)(nil),
		(*Truck)(nil),
	}
}

// a consumer side hosting method to relay wp creation notifications
func (ctx *consumerContext) TkCreated() {
	evtObj, err := ctx.Ho().CoRecvObj()
	if err != nil {
		glog.Error(err)
		return
	}
	wp, ok := evtObj.(*Truck)
	if !ok {
		err := errors.New(fmt.Sprintf("Sent a %T to TkCreated() ?!", evtObj))
		glog.Error(err)
		return
	}

	ctx.RLock() // RLock for proper sync
	defer ctx.RUnlock()

	cntNils := 0
	for i, n := 0, len(ctx.TkCreWatchers); i < n; i++ {
		ackCre := ctx.TkCreWatchers[i]
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
					ctx.TkCreWatchers[i] = nil
					cntNils++
				}
			}()
			if ackCre(wp) {
				// indicated stop by returning true, clear it
				ctx.TkCreWatchers[i] = nil
				cntNils++
			}
		}()
	}
	if cntNils > len(ctx.TkCreWatchers)/2 {
		// compact the slice to drive nils out, must start a new goro, as this
		// func currently holds a RLock, while `compactWatchers()` will WLock
		go ctx.compactWatchers()
	}
}

// a consumer side hosting method to relay wp move notifications
func (ctx *consumerContext) TkMoved(tid string, seq int, id string, x, y float64) {
	ctx.RLock() // RLock for proper sync
	defer ctx.RUnlock()

	cntNils := 0
	for i, n := 0, len(ctx.TkMoveWatchers); i < n; i++ {
		ackMv := ctx.TkMoveWatchers[i]
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
					ctx.TkMoveWatchers[i] = nil
					cntNils++
				}
			}()
			if ackMv(tid, seq, id, x, y) {
				// indicated stop by returning true, clear it
				ctx.TkMoveWatchers[i] = nil
				cntNils++
			}
		}()
	}
	if cntNils > len(ctx.TkMoveWatchers)/2 {
		// compact the slice to drive nils out, must start a new goro, as this
		// func currently holds a RLock, while `compactWatchers()` will WLock
		go ctx.compactWatchers()
	}
}

// a consumer side hosting method to relay wp move notifications
func (ctx *consumerContext) TkStopped(tid string, seq int, id string, moving bool) {
	ctx.RLock() // RLock for proper sync
	defer ctx.RUnlock()

	cntNils := 0
	for i, n := 0, len(ctx.TkStopWatchers); i < n; i++ {
		ackStop := ctx.TkStopWatchers[i]
		if ackStop == nil {
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
					ctx.TkStopWatchers[i] = nil
					cntNils++
				}
			}()
			if ackStop(tid, seq, id, moving) {
				// indicated stop by returning true, clear it
				ctx.TkStopWatchers[i] = nil
				cntNils++
			}
		}()
	}
	if cntNils > len(ctx.TkStopWatchers)/2 {
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
	for s, ckPos, n := ctx.TkCreWatchers, 0, len(ctx.TkCreWatchers); ckPos < n; ckPos++ {
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
		ctx.TkCreWatchers = ctx.TkCreWatchers[:owPos]
	}

	owPos = -1
	for s, ckPos, n := ctx.TkMoveWatchers, 0, len(ctx.TkMoveWatchers); ckPos < n; ckPos++ {
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
		ctx.TkMoveWatchers = ctx.TkMoveWatchers[:owPos]
	}

	owPos = -1
	for s, ckPos, n := ctx.TkStopWatchers, 0, len(ctx.TkStopWatchers); ckPos < n; ckPos++ {
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
		ctx.TkStopWatchers = ctx.TkStopWatchers[:owPos]
	}

}
