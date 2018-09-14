package backend

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"net/http"
)

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

	svc, err := svcs.GetRoutesService("", tid)
	if err != nil {
		panic(err)
	}

	func() { // WatchWaypoints() will call Notif(), will deadlock if called before co.Close()
		co, err := svc.Posting.Co()
		if err != nil {
			panic(err)
		}
		defer co.Close()
		result, err := co.Get(fmt.Sprintf(`
ListWaypoints(%#v)
`, tid))
		if err != nil {
			panic(err)
		}
		if e, ok := result.(error); ok {
			panic(e)
		}
		if e := wsc.WriteJSON(map[string]interface{}{
			"type": "initial",
			"wps":  result.(map[string]interface{})["wps"],
		}); e != nil {
			glog.Error(e)
			return
		}
	}()

	svc.HoCtx().(*routes.ConsumerContext).WatchWaypoints(tid, func(wp routes.Waypoint) (stop bool) {
		wp["_id"] = fmt.Sprintf("%s", wp["_id"]) // convert bson objId to str
		if e := wsc.WriteJSON(map[string]interface{}{
			"type": "created",
			"wp":   wp,
		}); e != nil {
			glog.Error(e)
			return true
		}
		return false
	}, func(id string, x, y float64) (stop bool) {
		if e := wsc.WriteJSON(map[string]interface{}{
			"type":  "moved",
			"wp_id": id, "x": x, "y": y,
		}); e != nil {
			glog.Error(e)
			return true
		}
		return false
	})
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
		return
	}

	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	var routesSvc *hbi.TCPConn
	routesSvc, err = svcs.GetRoutesService("", tid)
	if err != nil {
		return
	}

	// use async notification to cease round trips
	err = routesSvc.Posting.Notif(fmt.Sprintf(`
AddWaypoint(%#v,%#v,%#v)
`, tid, reqData.X, reqData.Y))
	if err != nil {
		return
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
		WpId string `json:"wp_id"`
		X, Y float64
	}
	decoder := json.NewDecoder(r.Body)
	if err = decoder.Decode(&reqData); err != nil {
		return
	}

	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	var routesSvc *hbi.TCPConn
	routesSvc, err = svcs.GetRoutesService("", tid)
	if err != nil {
		return
	}

	// use async notification to cease round trips
	if err = routesSvc.Posting.Notif(fmt.Sprintf(`
MoveWaypoint(%#v,%#v,%#v,%#v)
`, tid, reqData.WpId, reqData.X, reqData.Y)); err != nil {
		return
	}
}
