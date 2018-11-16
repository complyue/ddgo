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

func (wpc *wpcChgRelay) reload() (stop bool) {
	// fetch current snapshot of the whole collection
	ccn, wpl := wpc.routesAPI.FetchWaypoints()

	glog.V(1).Infof(" * wpc reloaded %v -> %v", wpc.ccn, ccn)
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

func (wpc *wpcChgRelay) Subscribed() (stop bool) {
	return wpc.reload()
}

func (wpc *wpcChgRelay) Epoch(ccn int) (stop bool) {
	glog.V(1).Infof(" ** Reloading wpc due to epoch CCN %v -> %v", wpc.ccn, ccn)
	return wpc.reload()
}

// Created
func (wpc *wpcChgRelay) MemberCreated(ccn int, eo livecoll.Member) (stop bool) {
	if ccnDistance := livecoll.ChgDistance(ccn, wpc.ccn); ccnDistance <= 0 {
		// ignore out-dated events
		return
	} else if ccnDistance > 1 {
		// event ccn is ahead of locally known ccn, reload
		glog.V(1).Infof(" ** Reloading wpc due to CCN changed %v -> %v", wpc.ccn, ccn)
		return wpc.reload()
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
	if ccnDistance := livecoll.ChgDistance(ccn, wpc.ccn); ccnDistance <= 0 {
		// ignore out-dated events
		return
	} else if ccnDistance > 1 {
		// event ccn is ahead of locally known ccn, reload
		glog.V(1).Infof(" ** Reloading wpc due to CCN changed %v -> %v", wpc.ccn, ccn)
		return wpc.reload()
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
func (wpc *wpcChgRelay) MemberDeleted(ccn int, id interface{}) (stop bool) {
	if ccnDistance := livecoll.ChgDistance(ccn, wpc.ccn); ccnDistance <= 0 {
		// ignore out-dated events
		return
	} else if ccnDistance > 1 {
		// event ccn is ahead of locally known ccn, reload
		glog.V(1).Infof(" ** Reloading wpc due to CCN changed %v -> %v", wpc.ccn, ccn)
		return wpc.reload()
	}

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
		}
	}()

	params := mux.Vars(r)
	tid := params["tid"]

	routesAPI, err := GetRoutesService(tid)
	if err != nil {
		panic(err)
	}
	subr := &wpcChgRelay{
		routesAPI: routesAPI, wsc: wsc, ccn: 0,
	}
	routesAPI.SubscribeWaypoints(subr)

	go func() {
		for {
			var msgIn map[string]interface{}
			if err := wsc.ReadJSON(&msgIn); err != nil {
				glog.Errorf("WS error: %+v", err)
				return
			}
			if len(msgIn) <= 0 {
				// keep alive
				routesAPI.EnsureAlive()
			} else {
				// todo other ops
			}
		}
	}()
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
