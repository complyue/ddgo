package backend

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo/bson"
	"github.com/golang/glog"
	"github.com/kataras/iris"
	"github.com/kataras/iris/websocket"
)

func showWaypoints() (ws *websocket.Server) {
	ws = websocket.New(websocket.Config{
		//BinaryMessages: true,
	})
	ws.OnConnection(func(c websocket.Connection) {
		tid := c.Context().Params().GetTrim("tid")
		defer func() {
			err := recover()
			if err != nil {
				errMsg := fmt.Sprintf("%+v", errors.RichError(err))
				glog.Error(errMsg)
				jsonBuf, err := json.Marshal(map[string]interface{}{
					"type": "err",
					"msg":  errMsg,
				})
				c.EmitMessage(jsonBuf)
				if err != nil {
					glog.Error(errors.RichError(err))
				}
				return
			}
		}()

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
			err = co.SendCode(fmt.Sprintf(`
ListWaypoints(%#v)
`, tid))
			if err != nil {
				panic(err)
			}
			result, err := co.RecvObj()
			if err != nil {
				panic(err)
			}
			if e, ok := result.(error); ok {
				panic(e)
			}
			jsonBuf, err := json.Marshal(map[string]interface{}{
				"type": "initial",
				"wps":  result.(bson.M)["wps"],
			})
			if err != nil {
				panic(err)
			}
			c.EmitMessage(jsonBuf)
		}()

		svc.HoCtx().(*routes.ConsumerContext).WatchWaypoints(tid, func(wp routes.Waypoint) (stop bool) {
			wp["_id"] = fmt.Sprintf("%s", wp["_id"]) // convert bson objId to str
			jsonBuf, err := json.Marshal(map[string]interface{}{
				"type": "created",
				"wp":   wp,
			})
			if err != nil {
				glog.Error(errors.RichError(err))
				c.Disconnect()
				return true
			}
			c.EmitMessage(jsonBuf)
			return
		}, func(id string, x, y float64) (stop bool) {
			jsonBuf, err := json.Marshal(map[string]interface{}{
				"type":  "moved",
				"wp_id": id, "x": x, "y": y,
			})
			if err != nil {
				glog.Error(errors.RichError(err))
				c.Disconnect()
				return true
			}
			c.EmitMessage(jsonBuf)
			return
		})

	})
	return
}

/*

async def show_waypoints(request: web.Request):
    """
    WebSocket connection to load all waypoints then watch changes to them.

    """
    ws = web.WebSocketResponse()
    await ws.prepare(request)

    try:
        tid = request.match_info['tid']

        # todo use cookie, csr token etc. for user/client tracking
        client_tracker = request.transport.get_extra_info('peername')

        async def wp_created(wp: dict):
            await ws.send_json({
                'type': 'created',
                'wp': wp,
            })

        async def wp_moved(wp_id: str, x: float, y: float):
            await ws.send_json({
                'type': 'moved',
                'wp_id': wp_id, 'x': x, 'y': y,
            })

        # tenant id is used both to separate service connections and
        # to select service proc (via sticky session)
        # TODO more fields to be used for service/session selection
        # Rules:
        #   multiple service connections can share a same session,
        #   but one service connection should stick to a sticky session
        async with routes_service(tid).co(session=tid) as svc:
            # register watchers and obtain list of all present
            # waypoints for this particular tenant
            await svc.context['register_watcher'](
                client_tracker, tid, wp_created, wp_moved,
            )
            wps = await svc.co_get(rf'''
list_waypoints({tid!r})
''')
            logger.debug(
                f'got {len(wps)} wps from {svc}'
            )

        logger.debug(f'sending wps list back ...')
        await ws.send_json({
            'type': 'initial',
            'wps': wps,
        })

        # wait until browser close the WebSocket
        msg = await ws.receive()
        if msg.type not in (ah.WSMsgType.CLOSE, ah.WSMsgType.CLOSING, ah.WSMsgType.CLOSED):
            logger.warning(f'WP ws msg from browser ?! {msg}')

    except (asyncio.CancelledError, futures.CancelledError):
        logger.debug(f'WP ws canceled.', exc_info=True)
    except asyncio.TimeoutError:
        logger.debug(f'WP ws timed out.', exc_info=True)
    except Exception as exc:
        logger.error(f'WP ws error.', exc_info=True)
        if not ws.closed:
            await ws.send_json({
                'type': 'err',
                'err': str(exc),
                'tb': traceback.format_exc(),
            })
    finally:
        if not ws.closed:
            await ws.close()

    return ws
*/

func addWaypoint(ctx iris.Context) {
	var err error
	defer func() {
		if err != nil {
			ctx.JSON(map[string]string{
				"err": fmt.Sprintf("%+v", errors.RichError(err)),
			})
		} else {
			// no error
			ctx.WriteString("{}")
		}
	}()
	tid := ctx.Params().GetTrim("tid")

	var reqData struct {
		X, Y float64
	}
	if err = ctx.ReadJSON(&reqData); err != nil {
		return
	}

	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	routesSvc, err := svcs.GetRoutesService("", tid)
	if err != nil {
		panic(err)
	}

	// use async notification to cease round trips
	if err = routesSvc.Posting.Notif(fmt.Sprintf(`
AddWaypoint(%#v,%#v,%#v)
`, tid, reqData.X, reqData.Y)); err != nil {
		return
	}

	return
}

func moveWaypoint(ctx iris.Context) {
	var err error
	defer func() {
		if err != nil {
			ctx.JSON(map[string]string{
				"err": fmt.Sprintf("%+v", errors.RichError(err)),
			})
		} else {
			// no error
			ctx.WriteString("{}")
		}
	}()
	tid := ctx.Params().GetTrim("tid")

	var reqData struct {
		WpId string
		X, Y float64
	}
	if err = ctx.ReadJSON(&reqData); err != nil {
		return
	}

	/*
		use tid as session for tenant isolation,
		and tunnel can further be specified to isolate per tenant or per other means
	*/
	routesSvc, err := svcs.GetRoutesService("", tid)
	if err != nil {
		panic(err)
	}

	// use async notification to cease round trips
	if err = routesSvc.Posting.Notif(fmt.Sprintf(`
MoveWaypoint(%#v,%#v,%#v,%#v)
`, tid, reqData.WpId, reqData.X, reqData.Y)); err != nil {
		return
	}

	return
}
