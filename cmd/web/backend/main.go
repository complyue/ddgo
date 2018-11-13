package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/complyue/ddgo/pkg/backend"
	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
)

func init() {
	// change glog default destination to stderr
	if glog.V(0) { // should always be true, mention glog so it defines its flags before we change them
		if err := flag.CommandLine.Set("logtostderr", "true"); nil != err {
			log.Printf("Failed changing glog default desitination, err: %s", err)
		}
	}
}

var mono bool
var devMode bool

func init() {
	flag.BoolVar(&mono, "mono", false, "Run in monolith mode.")
	flag.BoolVar(&devMode, "dev", false, "Run in development mode.")
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

	if mono {
		// monolith mode, create embedded consumer api objects,
		// and monkey patch consuming packages to use them

		backend.InitRoutesService = func(tid string) (*routes.ConsumerAPI, error) {
			return routes.NewMonoAPI(tid), nil
		}
		drivers.InitRoutesService = func(tid string) (*routes.ConsumerAPI, error) {
			return routes.NewMonoAPI(tid), nil
		}

		backend.InitDriversService = func(tid string) (*drivers.ConsumerAPI, error) {
			return drivers.NewMonoAPI(tid), nil
		}

	}

	webCfg, err := svcs.GetServiceConfig("web")
	if err != nil {
		return
	}

	router := mux.NewRouter()

	backend.DefineHttpRoutes(router)

	router.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static"))),
	)

	definePageRoutes(router)

	srv := &http.Server{
		Handler:      router,
		Addr:         webCfg.Http,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	addr := srv.Addr
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	glog.Infof("DDGo web serving at http://%s ...\n", ln.Addr())
	err = srv.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
}

// following copied from std lib as unexported

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
