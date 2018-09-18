package backend

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/ddgo/pkg/drivers"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
)

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

	driversApi, err := GetDriversService("", tid)
	if err != nil {
		panic(err)
	}

	tkl, err := driversApi.ListTrucks(tid)
	if err != nil {
		panic(err)
	}
	if e := wsc.WriteJSON(map[string]interface{}{
		"type":   "initial",
		"trucks": tkl.Trucks,
	}); e != nil {
		panic(errors.RichError(e))
	}

	var muWsc sync.Mutex

	driversApi.WatchTrucks(tid, func(tk *drivers.Truck) (stop bool) {
		muWsc.Lock()
		defer muWsc.Unlock()
		if e := wsc.WriteJSON(map[string]interface{}{
			"type":  "created",
			"truck": tk,
		}); e != nil {
			glog.Error(e)
			return true
		}
		return false
	}, func(tid string, seq int, id string, x, y float64) (stop bool) {
		muWsc.Lock()
		defer muWsc.Unlock()
		if e := wsc.WriteJSON(map[string]interface{}{
			"type": "moved",
			"tid":  tid, "seq": seq, "_id": id, "x": x, "y": y,
		}); e != nil {
			glog.Error(e)
			return true
		}
		return false
	}, func(tid string, seq int, id string, moving bool) (stop bool) {
		muWsc.Lock()
		defer muWsc.Unlock()
		if e := wsc.WriteJSON(map[string]interface{}{
			"type": "stopped",
			"tid":  tid, "seq": seq, "_id": id, "moving": moving,
		}); e != nil {
			glog.Error(e)
			return true
		}
		return false
	})

	// kickoff drivers team TODO find a better place to do this
	driversApi.DriversKickoff(tid)

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

	driversApi, err := GetDriversService("", tid)
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
		return
	}

	driversApi, err := GetDriversService("", tid)
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
		return
	}

	driversApi, err := GetDriversService("", tid)
	if err != nil {
		panic(err)
	}

	err = driversApi.StopTruck(tid, reqData.Seq, reqData.Id, reqData.Moving)
	if err != nil {
		panic(err)
	}
}
