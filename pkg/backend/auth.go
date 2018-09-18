package backend

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"net/http"
)

func authenticateUser(w http.ResponseWriter, r *http.Request) {
	var err error
	result := map[string]interface{}{}
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		if e := recover(); err != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(err)
			result["err"] = fmt.Sprintf("%+v", err)
		}
		buf, e := json.Marshal(result)
		if e != nil {
			glog.Error(e)
			return
		}
		w.Write(buf)
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	var reqData struct {
		Uid, Pwd, Tkn string
	}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&reqData)
	if err != nil {
		panic(err)
	}

	authApi, err := GetAuthService()
	if err != nil {
		panic(err)
	}

	newTkn, err := authApi.AuthenticateUser(tid, reqData.Uid, reqData.Pwd, reqData.Tkn)
	if err != nil {
		panic(err)
	}

	result["tkn"] = newTkn
}

func registerUser(w http.ResponseWriter, r *http.Request) {
	var err error
	result := map[string]interface{}{}
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		if e := recover(); err != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(err)
			result["err"] = fmt.Sprintf("%+v", err)
		}
		buf, e := json.Marshal(result)
		if e != nil {
			glog.Error(e)
			return
		}
		w.Write(buf)
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	var reqData struct {
		Uid, Pwd string
	}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&reqData)
	if err != nil {
		panic(err)
	}

	authApi, err := GetAuthService()
	if err != nil {
		panic(err)
	}

	ok, err := authApi.RegisterUser(tid, reqData.Uid, reqData.Pwd)
	if err != nil {
		panic(err)
	}

	result["ok"] = ok
}
