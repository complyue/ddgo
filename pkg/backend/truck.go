package backend

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/ddgo/pkg/livecoll"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// relay live truck collection changes over a websocket
type tkcChgRelay struct {
	driversAPI *drivers.ConsumerAPI // consuming api to drivers service
	wsc        *websocket.Conn      // the websocket connection
	ccn        int                  // known change number of the live truck collection
}

func (tkc *tkcChgRelay) Epoch(ccn int) (stop bool) {
	// fetch current snapshot of the whole collection
	ccn, tkl := tkc.driversAPI.FetchTrucks()

	tkc.ccn = ccn

	if e := tkc.wsc.WriteJSON(map[string]interface{}{
		"type":   "initial",
		"trucks": tkl,
	}); e != nil {
		glog.Error(errors.RichError(e))
		return true
	}

	return
}

// Created
func (tkc *tkcChgRelay) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, tkc.ccn) { // ignore out-dated events
		return
	}
	tk := eo.(*drivers.Truck)

	tkc.ccn = ccn

	if e := tkc.wsc.WriteJSON(map[string]interface{}{
		"type":  "created",
		"truck": tk,
	}); e != nil {
		glog.Error(e)
		return true
	}

	return
}

// Updated
func (tkc *tkcChgRelay) MemberUpdated(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, tkc.ccn) { // ignore out-dated events
		return
	}
	tk := eo.(*drivers.Truck)

	tkc.ccn = ccn

	// TODO distinguish move/stop
	if e := tkc.wsc.WriteJSON(map[string]interface{}{
		"type": "moved",
		"tid":  tkc.driversAPI.Tid(), "seq": tk.Seq, "_id": tk.Id, "x": tk.X, "y": tk.Y,
	}); e != nil {
		glog.Error(e)
		return true
	}
	if e := tkc.wsc.WriteJSON(map[string]interface{}{
		"type": "stopped",
		"tid":  tkc.driversAPI.Tid(), "seq": tk.Seq, "_id": tk.Id, "moving": tk.Moving,
	}); e != nil {
		glog.Error(e)
		return true
	}

	return
}

// Deleted
func (tkc *tkcChgRelay) MemberDeleted(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, tkc.ccn) { // ignore out-dated events
		return
	}
	// tk := eo.(*drivers.Truck)

	tkc.ccn = ccn

	// todo notify delete event

	return
}

func showTrucks(w http.ResponseWriter, r *http.Request) {
	var err error

	var wsc *websocket.Conn
	wsc, err = wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		glog.Error(errors.RichError(err))
		return
	}

	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			err = errors.RichError(err)
			glog.Error(err)
			if e := wsc.WriteJSON(map[string]interface{}{
				"type": "err",
				"msg":  fmt.Sprintf("%+v", err),
			}); e != nil {
				glog.Error(e)
				return
			}
		}
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	driversAPI, err := GetDriversService(tid)
	if err != nil {
		panic(err)
	}
	driversAPI.SubscribeTrucks(&tkcChgRelay{driversAPI: driversAPI, wsc: wsc})

	// kickoff drivers team TODO find a better place to do this
	driversAPI.DriversKickoff(tid)

}

func addTruck(w http.ResponseWriter, r *http.Request) {
	var err error
	result := map[string]interface{}{}
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(err)
			result["err"] = fmt.Sprintf("%+v", err)
		}
		buf, e := json.Marshal(result)
		if e != nil {
			panic(e)
		}
		w.Write(buf)
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	var reqData struct {
		X, Y float64
	}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&reqData)
	if err != nil {
		panic(err)
	}

	driversApi, err := GetDriversService(tid)
	if err != nil {
		panic(err)
	}

	err = driversApi.AddTruck(tid, reqData.X, reqData.Y)
	if err != nil {
		panic(err)
	}
}

func moveTruck(w http.ResponseWriter, r *http.Request) {
	var err error
	result := map[string]interface{}{}
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(err)
			result["err"] = fmt.Sprintf("%+v", err)
		}
		buf, e := json.Marshal(result)
		if e != nil {
			panic(e)
		}
		w.Write(buf)
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	var reqData struct {
		Seq  int
		Id   string `json:"_id"`
		X, Y float64
	}
	decoder := json.NewDecoder(r.Body)
	if err = decoder.Decode(&reqData); err != nil {
		panic(err)
	}

	driversApi, err := GetDriversService(tid)
	if err != nil {
		panic(err)
	}

	err = driversApi.MoveTruck(tid, reqData.Seq, reqData.Id, reqData.X, reqData.Y)
	if err != nil {
		panic(err)
	}
}

func stopTruck(w http.ResponseWriter, r *http.Request) {
	var err error
	result := map[string]interface{}{}
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		}
		if err != nil {
			glog.Error(err)
			result["err"] = fmt.Sprintf("%+v", err)
		}
		buf, e := json.Marshal(result)
		if e != nil {
			panic(e)
		}
		w.Write(buf)
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	var reqData struct {
		Seq    int
		Id     string `json:"_id"`
		Moving bool
	}
	decoder := json.NewDecoder(r.Body)
	if err = decoder.Decode(&reqData); err != nil {
		panic(err)
	}

	driversApi, err := GetDriversService(tid)
	if err != nil {
		panic(err)
	}

	err = driversApi.StopTruck(tid, reqData.Seq, reqData.Id, reqData.Moving)
	if err != nil {
		panic(err)
	}
}
