package backend

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/complyue/ddgo/pkg/livecoll"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// relay live waypoint collection changes over a websocket
type wpcChgRelay struct {
	routesAPI *routes.ConsumerAPI // consuming api to routes service
	wsc       *websocket.Conn     // the websocket connection
	ccn       int                 // known change number of the live waypoint collection
}

func (wpc *wpcChgRelay) Epoch(ccn int) (stop bool) {
	// fetch current snapshot of the whole collection
	ccn, wpl := wpc.routesAPI.FetchWaypoints()

	wpc.ccn = ccn

	if e := wpc.wsc.WriteJSON(map[string]interface{}{
		"type": "initial",
		"wps":  wpl,
	}); e != nil {
		glog.Error(errors.RichError(e))
		return true
	}

	return
}

// Created
func (wpc *wpcChgRelay) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, wpc.ccn) { // ignore out-dated events
		return
	}
	wp := eo.(*routes.Waypoint)

	wpc.ccn = ccn

	if e := wpc.wsc.WriteJSON(map[string]interface{}{
		"type": "created",
		"wp":   wp,
	}); e != nil {
		glog.Error(e)
		return true
	}

	return
}

// Updated
func (wpc *wpcChgRelay) MemberUpdated(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, wpc.ccn) { // ignore out-dated events
		return
	}
	wp := eo.(*routes.Waypoint)

	wpc.ccn = ccn

	if e := wpc.wsc.WriteJSON(map[string]interface{}{
		"type": "moved",
		"tid":  wpc.routesAPI.Tid(), "seq": wp.Seq, "_id": wp.Id, "x": wp.X, "y": wp.Y,
	}); e != nil {
		glog.Error(e)
		return true
	}

	return
}

// Deleted
func (wpc *wpcChgRelay) MemberDeleted(ccn int, eo livecoll.Member) (stop bool) {
	if livecoll.IsOld(ccn, wpc.ccn) { // ignore out-dated events
		return
	}
	// wp := eo.(*routes.Waypoint)

	wpc.ccn = ccn

	// todo notify delete event

	return
}

func showWaypoints(w http.ResponseWriter, r *http.Request) {
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

	routesAPI, err := GetRoutesService(tid)
	if err != nil {
		panic(err)
	}
	routesAPI.SubscribeWaypoints(&wpcChgRelay{routesAPI: routesAPI, wsc: wsc})
}

func addWaypoint(w http.ResponseWriter, r *http.Request) {
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

	routesApi, err := GetRoutesService(tid)
	if err != nil {
		panic(err)
	}

	err = routesApi.AddWaypoint(tid, reqData.X, reqData.Y)
	if err != nil {
		panic(err)
	}
}

func moveWaypoint(w http.ResponseWriter, r *http.Request) {
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

	routesApi, err := GetRoutesService(tid)
	if err != nil {
		panic(err)
	}

	err = routesApi.MoveWaypoint(tid, reqData.Seq, reqData.Id, reqData.X, reqData.Y)
	if err != nil {
		panic(err)
	}
}
