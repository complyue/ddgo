package backend

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/ddgo/pkg/routes"
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

	routesApi, err := GetRoutesService("", tid)
	if err != nil {
		panic(err)
	}

	wpl, err := routesApi.ListWaypoints(tid)
	if err != nil {
		panic(err)
	}
	if e := wsc.WriteJSON(map[string]interface{}{
		"type": "initial",
		"wps":  wpl.Waypoints,
	}); e != nil {
		panic(errors.RichError(e))
	}

	routesApi.WatchWaypoints(tid, func(wp *routes.Waypoint) (stop bool) {
		if e := wsc.WriteJSON(map[string]interface{}{
			"type": "created",
			"wp":   wp,
		}); e != nil {
			glog.Error(e)
			return true
		}
		return false
	}, func(tid string, seq int, id string, x, y float64) (stop bool) {
		if e := wsc.WriteJSON(map[string]interface{}{
			"type": "moved",
			"tid":  tid, "seq": seq, "_id": id, "x": x, "y": y,
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

	routesApi, err := GetRoutesService("", tid)
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

	routesApi, err := GetRoutesService("", tid)
	if err != nil {
		panic(err)
	}

	err = routesApi.MoveWaypoint(tid, reqData.Seq, reqData.Id, reqData.X, reqData.Y)
	if err != nil {
		panic(err)
	}
}
