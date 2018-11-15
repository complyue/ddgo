package dbc

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo"
	"github.com/golang/glog"
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

	for db == nil {
		servicesEtc, e := ioutil.ReadFile("etc/services.json")
		if e != nil {
			cwd, _ := os.Getwd()
			err = errors.Wrapf(e, "Can NOT read etc/services.json`, [%s] may not be the right directory ?\n", cwd)
			return nil
		}

		svcConfigs := make(map[string]struct {
			Url string
		})
		err = json.Unmarshal(servicesEtc, &svcConfigs)
		if err != nil {
			err = errors.Wrap(err, "Failed parsing services.json")
			return nil
		}

		dbConfig := svcConfigs["db"]

		session, err = mgo.Dial(dbConfig.Url)
		if err != nil {
			return nil
		}
		db = session.DB("dd")
	}

	return db
}
