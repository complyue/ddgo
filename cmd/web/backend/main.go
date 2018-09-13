package main

import (
	"flag"
	"github.com/complyue/ddgo/pkg/backend"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"log"
	"net/http"
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

	router := mux.NewRouter()

	backend.DefineHttpRoutes(router)

	router.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static"))),
	)

	definePageRoutes(router)

	srv := &http.Server{
		Handler:      router,
		Addr:         ":7070",
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	err = srv.ListenAndServe()
}
