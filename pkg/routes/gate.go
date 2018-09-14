package routes

import (
	"fmt"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo/bson"
	"github.com/golang/glog"
)

func NewServiceContext() hbi.HoContext {
	return &ServiceContext{
		HoContext: hbi.NewHoContext(),
	}
}

type ServiceContext struct {
	hbi.HoContext
}

// expose as a service method in rpc style
func (ctx *ServiceContext) ListWaypoints(tid string) {
	p2p := ctx.PoToPeer()
	wps, err := ListWaypoints(tid)
	if err != nil {
		glog.Error(errors.RichError(err))
		p2p.CoSendCode(fmt.Sprintf(`
NewError(%#v)
`, err.Error()))
		return
	}
	if err = p2p.CoSendBSON(bson.M{
		"wps": wps,
	}); err != nil {
		panic(errors.RichError(err))
	}
}

// implement sub func as a service method
func (ctx *ServiceContext) WatchWaypoints(tid string) {
	WatchWaypoints(tid, func(wp Waypoint) (stop bool) {
		defer func() {
			if err := recover(); err != nil {
				stop = true
				glog.Error(errors.RichError(err))
			}
		}()
		p2p := ctx.PoToPeer()
		if p2p.Cancelled() {
			return true
		}
		_id := wp["_id"].(bson.ObjectId).Hex()
		wp["_id"] = _id // convert oid to hex str for frontend
		buf, err := bson.Marshal(wp)
		if err != nil {
			glog.Error(errors.RichError(err))
			return true
		}
		// convey wp data by a binary stream
		bc := make(chan []byte, 1)
		bc <- buf
		close(bc)
		p2p.NotifCoRun(fmt.Sprintf(`
WpCreatedB(%#v,%#v)
`, tid, len(buf)), bc)
		return
	}, func(id string, x, y float64) (stop bool) {
		defer func() {
			if err := recover(); err != nil {
				stop = true
				glog.Error(errors.RichError(err))
			}
		}()
		p2p := ctx.PoToPeer()
		if p2p.Cancelled() {
			return true
		}
		if err := p2p.Notif(fmt.Sprintf(`
WpMoved(%#v,%#v,%#v,%#v)
`, tid, id, x, y)); err != nil {
			return true
		}
		return
	})
}

// expose as a service method in notif style
func (ctx *ServiceContext) MoveWaypoint(tid string, id string, x, y float64) (err error) {
	if err = MoveWaypoint(tid, id, x, y); err != nil {
		glog.Error(errors.RichError(err))
		// todo async notify api gateway
		return
	}
	return
}

// expose as a service method in notif style
func (ctx *ServiceContext) AddWaypoint(tid string, x, y float64) (err error) {
	if err = AddWaypoint(tid, x, y); err != nil {
		glog.Error(errors.RichError(err))
		// todo async notify api gateway
		return
	}
	return
}

func NewConsumerContext() hbi.HoContext {
	ctx := &ConsumerContext{
		HoContext: hbi.NewHoContext(),
	}
	return ctx
}

type ConsumerContext struct {
	hbi.HoContext

	WatchedTid     string
	WpCreWatchers  []func(wp Waypoint) bool
	WpMoveWatchers []func(id string, x, y float64) bool
}

// this is not a hosting method, but intended to be used by the service consumer,
// to register watchers subscribing the relevant domain events.
func (ctx *ConsumerContext) WatchWaypoints(
	tid string,
	ackCre func(wp Waypoint) bool,
	ackMv func(id string, x, y float64) bool,
) {
	if ctx.WatchedTid == "" {
		ctx.PoToPeer().Notif(fmt.Sprintf(`
WatchWaypoints(%#v)
`, tid))
		ctx.WatchedTid = tid
	} else if tid != ctx.WatchedTid {
		panic(errors.New(fmt.Sprintf("request to watch tid=%s while already watched %s ?!", tid, ctx.WatchedTid)))
	}
	// todo besides append, can linear search a nil slot to put in
	if ackCre != nil {
		ctx.WpCreWatchers = append(ctx.WpCreWatchers, ackCre)
	}
	if ackMv != nil {
		ctx.WpMoveWatchers = append(ctx.WpMoveWatchers, ackMv)
	}
}

// a hosting method to relay wp creation notifications
func (ctx *ConsumerContext) WpCreatedB(tid string, bufLen int) {
	if tid != ctx.WatchedTid {
		glog.Errorf("Got WpCreatedB for tid=%s while watching %s ?!", tid, ctx.WatchedTid)
		return
	}
	buf := make([]byte, bufLen)
	bc := make(chan []byte, 1)
	bc <- buf
	close(bc)
	ctx.HoContext.Ho().CoRecvData(bc)
	if ctx.WpCreWatchers == nil {
		// no watcher at all, but why got notification ?
		return
	}
	var wp Waypoint
	err := bson.Unmarshal(buf, &wp)
	if err != nil {
		glog.Error(errors.RichError(err))
		return
	}
	for i, n := 0, len(ctx.WpCreWatchers); i < n; i++ {
		ackCre := ctx.WpCreWatchers[i]
		if ackCre == nil {
			// already cleared
			continue
		}
		func() {
			defer func() {
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
					// clear on error
					ctx.WpCreWatchers[i] = nil
				}
			}()
			if ackCre(wp) {
				// indicated stop by returning true, clear it
				ctx.WpCreWatchers[i] = nil
			}
		}()
	}
}

// a hosting method to relay wp move notifications
func (ctx *ConsumerContext) WpMoved(tid string, id string, x, y float64) {
	if tid != ctx.WatchedTid {
		glog.Errorf("Got WpMoved for tid=%s while watched %s ?!", tid, ctx.WatchedTid)
		return
	}
	if ctx.WpMoveWatchers == nil {
		// no watcher at all, but why got notification ?
		return
	}
	for i, n := 0, len(ctx.WpMoveWatchers); i < n; i++ {
		ackMv := ctx.WpMoveWatchers[i]
		if ackMv == nil {
			// already cleared
			continue
		}
		func() {
			defer func() {
				err := recover()
				if err != nil {
					glog.Error(errors.RichError(err))
					// clear on error
					ctx.WpMoveWatchers[i] = nil
				}
			}()
			if ackMv(id, x, y) {
				// indicated stop by returning true, clear it
				ctx.WpMoveWatchers[i] = nil
			}
		}()
	}
}
