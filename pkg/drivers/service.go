// define the service interface with types & methods exposed for consumption.
package drivers

import (
	"fmt"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
)

// construct a service hosting context for serving over HBI wires
func NewServiceContext() hbi.HoContext {
	return &serviceContext{
		HoContext: hbi.NewHoContext(),
	}
}

// implementation details of service context
type serviceContext struct {
	hbi.HoContext
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *serviceContext) TypesToExpose() []interface{} {
	return []interface{}{
		(*TruckList)(nil),
		(*Truck)(nil),
	}
}

// this service method has rpc style, with err-out converted to panic,
// which will induce forceful disconnection
func (ctx *serviceContext) ListTrucks(tid string) *TruckList {
	wpl, err := ListTrucks(tid)
	if err != nil {
		panic(err)
	}
	return wpl
}

// this service method to subscribe waypoint events per the specified tid
func (ctx *serviceContext) WatchTrucks(tid string) {
	WatchTrucks(tid, func(wp *Truck) (stop bool) {
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
		p2p.NotifBSON(`
TkCreated()
`, wp, "&Truck{}")
		return
	}, func(tid string, seq int, id string, x, y float64) (stop bool) {
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
TkMoved(%#v,%#v,%#v,%#v,%#v)
`, tid, seq, id, x, y)); err != nil {
			return true
		}
		return
	}, func(tid string, seq int, id string, moving bool) (stop bool) {
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
TkStopped(%#v,%#v,%#v,%#v)
`, tid, seq, id, moving)); err != nil {
			return true
		}
		return
	})
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) AddTruck(tid string, x, y float64) error {
	return AddTruck(tid, x, y)
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) MoveTruck(
	tid string, seq int, id string, x, y float64,
) error {
	return MoveTruck(tid, seq, id, x, y)
}

// this service method has async style, successful result will be published
// as an event asynchronously
func (ctx *serviceContext) StopTruck(
	tid string, seq int, id string, moving bool,
) error {
	return StopTruck(tid, seq, id, moving)
}
