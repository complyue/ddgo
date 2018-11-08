package main

import (
	"flag"
	"fmt"
	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/complyue/hbigo/pkg/svcpool"
	"github.com/golang/glog"
	"log"
	"net"
	"os"
	"runtime"
	"time"
)

func init() {
	// change glog default destination to stderr
	if glog.V(0) { // should always be true, mention glog so it defines its flags before we change them
		if err := flag.CommandLine.Set("logtostderr", "true"); nil != err {
			log.Printf("Failed changing glog default desitination, err: %+v", err)
		}
	}
}

var teamAddr string
var solo bool

func init() {

	flag.StringVar(&teamAddr, "team", "#", "service pool teaming address")

	flag.BoolVar(&solo, "solo", false, "Run in solo mode.")

}

func main() {
	var err error
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(errors.RichError(err))
		}
	}()

	flag.Parse()

	var poolConfig svcs.ServiceConfig
	poolConfig, err = svcs.GetServiceConfig("drivers")
	if err != nil {
		panic(err)
	}

	if solo {
		// started with -solo, run with embedded service registry always resolve to self

		if err = drivers.ServeSolo(); err != nil {
			glog.Error(err)
		} else {
			// block main goro forever
			<-(chan struct{})(nil)
		}

	} else if teamAddr == "#" {
		// started without -team, assume pool master

		glog.Infof("Starting drivers service pool with config: %+v\n", poolConfig)
		startProcessTimeout, err := time.ParseDuration(poolConfig.Timeout)
		if err != nil {
			return
		}

		var master *svcpool.Master
		master, err = svcpool.NewMaster(poolConfig.Size, poolConfig.Hot, startProcessTimeout)
		master.Serve(fmt.Sprintf("%s:%d", poolConfig.Host, poolConfig.Port))

	} else {
		// started with -team, assume proc worker process

		// apply parallelism config
		runtime.GOMAXPROCS(poolConfig.Parallel)
		glog.V(1).Infof(
			"Drivers service proc [pid=%d] parallelism set to %d by configured %d\n",
			os.Getpid(), runtime.GOMAXPROCS(0), poolConfig.Parallel,
		)

		glog.Infof("Drivers service proc [pid=%d,team=%s] starting ...", os.Getpid(), teamAddr)
		hbi.ServeTCP(drivers.NewServiceContext, fmt.Sprintf("%s:0", poolConfig.Host), func(listener *net.TCPListener) {
			glog.Infof("Drivers service proc [pid=%d,team=%s] listening %+v", os.Getpid(), teamAddr, listener.Addr())
			procPort := listener.Addr().(*net.TCPAddr).Port

			var m4w *hbi.TCPConn
			m4w, err = hbi.DialTCP(svcpool.NewWorkerHoContext(), teamAddr)
			p2p := m4w.MustPoToPeer()
			p2p.Notif(fmt.Sprintf(`
WorkerOnline(%#v,%#v,"")
`, os.Getpid(), procPort))
			glog.Infof("Drivers service proc [pid=%d,team=%s] reported to master.", os.Getpid(), teamAddr)
		})

	}

}
