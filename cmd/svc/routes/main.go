package main

import (
	"encoding/json"
	"flag"
	"fmt"
	. "github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/complyue/hbigo/pkg/svcpool"
	"github.com/golang/glog"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"
)

func init() {
	var err error

	// change glog default destination to stderr
	if glog.V(0) { // should always be true, mention glog so it defines its flags before we change them
		if err = flag.CommandLine.Set("logtostderr", "true"); nil != err {
			log.Printf("Failed changing glog default desitination, err: %s", err)
		}
	}

}

var teamAddr string

func init() {

	flag.StringVar(&teamAddr, "team", "#", "service pool teaming address")

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

	servicesEtc, err := ioutil.ReadFile("etc/services.json")
	if err != nil {
		cwd, _ := os.Getwd()
		panic(errors.Wrapf(err, "Can NOT read etc/services.json`, [%s] may not be the right directory ?\n", cwd))
	}

	svcConfigs := make(map[string]struct {
		Host    string
		Port    int
		Size    int
		Hot     int
		Timeout string
	})
	err = json.Unmarshal(servicesEtc, &svcConfigs)
	if err != nil {
		panic(errors.Wrap(err, "Failed parsing services.json"))
	}

	poolConfig := svcConfigs["routes"]

	if teamAddr == "#" {
		// started without -team, assume pool master

		glog.Infof("Starting routes service pool with config: %+v\n", poolConfig)
		startProcessTimeout, err := time.ParseDuration(poolConfig.Timeout)
		if err != nil {
			return
		}

		var master *svcpool.Master
		master, err = svcpool.NewMaster(poolConfig.Size, poolConfig.Hot, startProcessTimeout)
		master.Serve(fmt.Sprintf("%s:%d", poolConfig.Host, poolConfig.Port))

	} else {
		// started with -team, assume proc worker process

		glog.Infof("Routes service proc [pid=%d,team=%s] starting ...", os.Getpid(), teamAddr)
		hbi.ServeTCP(NewServiceContext, fmt.Sprintf("%s:0", poolConfig.Host), func(listener *net.TCPListener) {
			glog.Infof("Routes service proc [pid=%d] listening %+v", os.Getpid(), listener.Addr())
			procPort := listener.Addr().(*net.TCPAddr).Port

			var m4w *hbi.TCPConn
			m4w, err = hbi.DialTCP(svcpool.NewWorkerHoContext(), teamAddr)
			p2p := m4w.PoToPeer()
			p2p.Notif(fmt.Sprintf(`
WorkerOnline(%#v,%#v)
`, os.Getpid(), procPort))
			glog.Infof("Routes service proc [pid=%d] reported to master at [%s]", os.Getpid(), teamAddr)
		})

	}

}
