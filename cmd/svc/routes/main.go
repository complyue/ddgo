package main

import (
	"encoding/json"
	"flag"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"io/ioutil"
	"log"
	"os"
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

var teamPort int

func init() {

	flag.IntVar(&teamPort, "team", 0, "Teaming port")

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

	if teamPort <= 0 {
		// started directly, assume team master

		//img, err := os.Executable()
		//if nil != err {
		//	log.Fatal("No executable?!")
		//}

		servicesEtc, err := ioutil.ReadFile("etc/services.json")
		if err != nil {
			cwd, _ := os.Getwd()
			panic(errors.Wrapf(err, "Can NOT read etc/services.json`, [%s] may not be the right directory ?\n", cwd))
		}

		svcAddrs := make(map[string]struct {
			Host string
			Port int
		})
		err = json.Unmarshal(servicesEtc, &svcAddrs)
		if err != nil {
			panic(errors.Wrap(err, "Failed parsing services.json"))
		}

		glog.Infof("Starting routes service at %+v\n", svcAddrs["routes"])

	}

}
