package backend

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/kataras/iris"
	"github.com/kataras/iris/websocket"
)

func showWaypoints() (ws *websocket.Server) {
	ws = websocket.New(websocket.Config{
		//BinaryMessages: true,
	})

	ws.OnConnection(func(c websocket.Connection) {
		tid := c.Context().Params().GetTrim("tid")

		c.OnMessage(func(data []byte) {
			message := string(data)
		})

		c.OnDisconnect(func() {
		})

	})
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
	tid := ctx.Params().GetTrim("tid")

	var reqData struct {
		X, Y float64
	}
	if err := ctx.ReadJSON(&reqData); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.WriteString(err.Error())
		return
	}

	/*
		use tid as session for tenant isolation,
		and wireKey can further be used to isolate communication channels,
		per tenant or per other means
	*/
	routesSvc, err := svcs.GetService("routes", "", tid)
	if err != nil {
		panic(err)
	}

	co, err := routesSvc.Co()
	if err != nil {
		panic(err)
	}
	defer co.Close()

	wp, err := co.Get(fmt.Sprintf(`
AddWaypoint(%#v,%#v%#v)
`, tid, reqData.X, reqData.Y))
	if err != nil {
		panic(err)
	}

	ctx.JSON(map[string]interface{}{
		"data": json.Marshal(wp),
	})
}

func moveWaypoint(ctx iris.Context) {
	tid := ctx.Params().GetTrim("tid")

	var reqData struct {
		WpId string
		X, Y float64
	}
	if err := ctx.ReadJSON(&reqData); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.WriteString(err.Error())
		return
	}

	/*
		use tid as session for tenant isolation,
		and wireKey can further be used to isolate communication channels,
		per tenant or per other means
	*/
	routesSvc, err := svcs.GetService("routes", "", tid)
	if err != nil {
		panic(err)
	}

	co, err := routesSvc.Co()
	if err != nil {
		panic(err)
	}
	defer co.Close()

	moveResult, err := co.Get(fmt.Sprintf(`
MoveWaypoint(%#v,%#v,%#v%#v)
`, tid, reqData.WpId, reqData.X, reqData.Y))
	if err != nil {
		panic(err)
	}

	ctx.JSON(map[string]interface{}{
		"data": json.Marshal(moveResult.(struct {
			X, Y float64
		})),
	})
}
