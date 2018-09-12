package dbc

import (
	"encoding/json"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo"
	"github.com/golang/glog"
	"io/ioutil"
	"os"
)

var session *mgo.Session
var db *mgo.Database

func DB() *mgo.Database {
	var err error
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(errors.RichError(err))
			panic(err)
		}
	}()

	if db == nil {
		servicesEtc, err := ioutil.ReadFile("etc/services.json")
		if err != nil {
			cwd, _ := os.Getwd()
			panic(errors.Wrapf(err, "Can NOT read etc/services.json`, [%s] may not be the right directory ?\n", cwd))
		}

		svcConfigs := make(map[string]struct {
			Url string
		})
		err = json.Unmarshal(servicesEtc, &svcConfigs)
		if err != nil {
			panic(errors.Wrap(err, "Failed parsing services.json"))
		}

		dbConfig := svcConfigs["db"]

		session, err = mgo.Dial(dbConfig.Url)
		if err != nil {
			return
		}
		session.DB("dd")
	}

	return db
}
