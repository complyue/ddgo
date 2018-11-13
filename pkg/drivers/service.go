// define the service interface with types & methods exposed for consumption.
package drivers

import (
	"fmt"
	"net"
	"os"

	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/svcpool"
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
		(*Truck)(nil),
	}
}

func ServeSolo() error {
	var poolConfig svcs.ServiceConfig
	poolConfig, err := svcs.GetServiceConfig("drivers")
	if err != nil {
		return err
	}
	// started with an embedded service registry always resolve to self
	soloHost, soloPort := poolConfig.Host, poolConfig.Port
	procAddr := fmt.Sprintf("%s:%d", soloHost, soloPort)
	glog.Infof("Drivers service solo proc [pid=%d] starting ...", os.Getpid())
	go hbi.ServeTCP(
		func() hbi.HoContext {
			type SoloCtx struct {
				hbi.HoContext
				svcpool.StaticRegistry
			}
			return &SoloCtx{NewServiceContext(),
				svcpool.StaticRegistry{ServiceAddr: procAddr}}
		}, procAddr, func(listener *net.TCPListener) {
			glog.Infof("Drivers service solo proc [pid=%d] listening %+v",
				os.Getpid(), listener.Addr())
		},
	)
	return nil
}
