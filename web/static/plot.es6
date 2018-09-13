(() => {
    const showArea = $('#show_area'),
        waypointTmpl = $('#tmpl .WayPoint'), truckTmpl = $('#tmpl .Truck');
    let wpById = {}, truckById = {};

    (function showWaypointsLive() {
        let ws = window.wpWatcher;
        if (ws) {
            if (WebSocket.CONNECTING === ws.readyState || WebSocket.OPEN === ws.readyState) {
                console.error('repeated wp watching ws ?!');
                return;
            }
        }
        // establish ws to load initial list of wps and receive further updates
        ws = window.wpWatcher = new WebSocket('ws://' + location.host
            + '/api/' + window.tid + '/waypoint',
        );
        ws.onopen = function () {
        };
        ws.onmessage = (me) => {
            if ('string' !== typeof me.data) {
                throw 'WS msg of type ' + typeof me.data + ' ?!';
            }

            let result = JSON.parse(me.data);
            if ('err' === result.type) {
                console.error('WP watching error:', result);
                debugger;
                alert('Waypoint watching error: ' + result.msg);
                // reconnect in 10 seconds
                setTimeout(() => {
                    ws.close();
                    showWaypointsLive();
                }, 10000);
            } else if ('initial' === result.type) {

                showArea.find('.WayPoint').remove();
                wpById = {};
                for (let {_id, seq, label, x, y} of result.wps) {
                    let wp = waypointTmpl.clone();
                    wp.data({'wp_id': _id, 'seq': seq});
                    wp.find('.Label').text(label);
                    wp.appendTo(showArea);
                    wp.css({left: x, top: y});
                    wpById[_id] = wp;
                }

            } else if ('created' === result.type) {

                let {_id, seq, label, x, y} = result.wp;

                let wp = waypointTmpl.clone();
                wp.data({'wp_id': _id, 'seq': seq});
                wp.find('.Label').text(label);
                wp.appendTo(showArea);
                wp.css({left: x, top: y});
                wpById[_id] = wp;

            } else if ('moved' === result.type) {

                let {wp_id, x, y} = result;
                // show the movement use a straight line path.
                let wp = wpById[wp_id];
                wp.animate({left: x, top: y});

            } else {
                console.error('WP watching ws msg not understood:', result);
                debugger;
            }
        };
        ws.onclose = () => {
            console.error('WP watching ws closed!');
            // todo make sure reconnection scheduled, different delay or strategy
            // todo should be used for different cases
            // don't attempt a plain reconnection here
        };
        ws.onerror = (err) => {
            console.error('WP ws error:', err);
            debugger;
            // reconnect in 10 seconds
            setTimeout(() => {
                showWaypointsLive();
            }, 10000);
        };
    })();

    (function showTrucksLive(skip) {
        if (skip) return;

        let ws = window.truckWatcher;
        if (ws) {
            if (WebSocket.CONNECTING === ws.readyState || WebSocket.OPEN === ws.readyState) {
                console.error('repeated truck watching ws ?!');
                return;
            }
        }
        // establish ws to load initial list of trucks and receive further updates
        ws = window.truckWatcher = new WebSocket('ws://' + location.host
            + '/api/' + window.tid + '/truck',
        );
        ws.onopen = function () {
        };
        ws.onmessage = (me) => {
            if ('string' !== typeof me.data) {
                throw 'WS msg of type ' + typeof me.data + ' ?!';
            }

            let result = JSON.parse(me.data);
            if ('err' === result.type) {
                console.error('Truck watching error:', result);
                debugger;
                alert('Truck watching error: ' + result.msg);
                // reconnect in 10 seconds
                setTimeout(() => {
                    ws.close();
                    showTrucksLive();
                }, 10000);
            } else if ('initial' === result.type) {

                showArea.find('.Truck').remove();
                truckById = {};
                for (let {_id, seq, label, x, y, moving} of result.trucks) {
                    let truck = truckTmpl.clone();
                    truck.data({'truck_id': _id, 'seq': seq, 'moving': moving});
                    truck.find('.Label').text(label);
                    truck.appendTo(showArea);
                    truck.css({left: x, top: y});
                    truckById[_id] = truck;
                }

            } else if ('created' === result.type) {

                let {_id, seq, label, x, y, moving} = result.truck;

                let truck = truckTmpl.clone();
                truck.data({'truck_id': _id, 'seq': seq, 'moving': moving});
                truck.find('.Label').text(label);
                truck.appendTo(showArea);
                truck.css({left: x, top: y});
                truckById[_id] = truck;

            } else if ('moved' === result.type) {

                let {truck_id, x, y} = result;
                // show the movement use a straight line path.
                let truck = truckById[truck_id];
                truck.animate({left: x, top: y});

            } else if ('stopped' === result.type) {

                let {truck_id, moving} = result;
                let truck = truckById[truck_id];
                truck.data('moving', !!moving);

            } else {
                console.error('Truck watching ws msg not understood:', result);
                debugger;
            }
        };
        ws.onclose = () => {
            console.error('Truck watching ws closed!');
            // todo make sure reconnection scheduled, different delay or strategy
            // todo should be used for different cases
            // don't attempt a plain reconnection here
        };
        ws.onerror = (err) => {
            console.error('Truck ws error:', err);
            debugger;
            // reconnect in 10 seconds
            setTimeout(() => {
                showTrucksLive();
            }, 10000);
        };
    })(true);

    let draggedObj = null;

    $('body').mousedown('.Truck, .WayPoint', async function (me) {
        let clicked = $(me.target).closest('.Truck');
        if (!clicked.length) {
            clicked = $(me.target).closest('.WayPoint');
        }
        if (!clicked.length) {
            draggedObj = null;
            return nonDragClick(me);
        }

        if ($('#toggle_stop').prop('checked')) {
            // toggle stop

            me.stopImmediatePropagation();
            me.preventDefault();

            if (clicked.hasClass('Truck')) {
                // toggle truck stop

                let truck = clicked;
                let result = await $.ajax({
                    dataType: 'json', method: 'post', url: '/api/' + window.tid + '/truck/stop',
                    contentType: "application/json", data: JSON.stringify({
                        truck_id: truck.data('truck_id'), moving: !truck.data('moving'),
                    }),
                });
                if (result.err) {
                    console.error('backend returned error in result:', result);
                    debugger;
                    throw new Error(result.err);
                }

            } else if (clicked.hasClass('WayPoint')) {
                // todo toggle wp stop
            }

        } else {
            // initiate drag

            draggedObj = clicked;
            me.stopImmediatePropagation();
            me.preventDefault();

            let draggedAvatar = draggedObj.clone();
            draggedAvatar.css('opacity', draggedObj.css('opacity') / 2);
            draggedObj.data('dragInfo', {
                posOffset: draggedObj.position(), mouseX: me.pageX, mouseY: me.pageY,
                avatar: draggedAvatar,
            });
            draggedAvatar.insertBefore(draggedObj);

        }
    }).mousemove(function (me) {
        if (null === draggedObj) {
            return;
        }
        let dragInfo = draggedObj.data('dragInfo');
        if (!dragInfo) {
            draggedObj = null;
            return;
        }
        me.stopImmediatePropagation();
        me.preventDefault();

        let {posOffset, mouseX, mouseY, avatar} = dragInfo;
        let newX = posOffset.left + me.pageX - mouseX, newY = posOffset.top + me.pageY - mouseY;

        // todo draw an arrow line to illustrate the tobe movement
        avatar.css({left: newX, top: newY});
        // todo consider triggering backend actions during drag in realtime ?
    }).mouseup(async function (me) {
        if (null === draggedObj) {
            return;
        }
        me.stopImmediatePropagation();
        me.preventDefault();

        let {posOffset, mouseX, mouseY, avatar} = draggedObj.data('dragInfo');
        draggedObj.data('dragInfo', null); // clear drag info anyway
        let newX = posOffset.left + me.pageX - mouseX, newY = posOffset.top + me.pageY - mouseY;

        // call backend for movement of wp/truck
        try {
            if (draggedObj.hasClass('WayPoint')) {
                // call backend api of routes domain to actually move the waypoint dragged

                let wp = draggedObj;
                let result = await $.ajax({
                    dataType: 'json', method: 'post', url: '/api/' + window.tid + '/waypoint/move',
                    contentType: "application/json", data: JSON.stringify({
                        wp_id: wp.data('wp_id'), x: newX, y: newY,
                    }),
                });
                if (result.err) {
                    console.error('backend returned error in result:', result);
                    debugger;
                    throw new Error(result.err);
                }
                // move result should be passed over watching ws as notification

            } else if (draggedObj.hasClass('Truck')) {
                // todo call backend api of drivers domain to actually move the truck dragged

                let truck = draggedObj;
                let result = await $.ajax({
                    dataType: 'json', method: 'post', url: '/api/' + window.tid + '/truck/move',
                    contentType: "application/json", data: JSON.stringify({
                        truck_id: truck.data('truck_id'), x: newX, y: newY,
                    }),
                });
                if (result.err) {
                    console.error('backend returned error in result:', result);
                    debugger;
                    throw new Error(result.err);
                }
                // move result should be passed over watching ws as notification


            } else {
                console.trace('dragged object unknown ?!', draggedObj);
            }
        } catch (err) {
            console.error('backend error', err);
        } finally {
            avatar.remove();
        }

        draggedObj = null;
    });

    function nonDragClick(me) {
        let showArea = $(me.target).closest('.ShowArea');
        if (!showArea.length) {
            return;
        }
        let showAreaOffset = showArea.offset();
        let atX = me.pageX - showAreaOffset.left, atY = me.pageY - showAreaOffset.top;

        if ($('#plan_route').prop('checked')) {

            // call backend to add the wp
            $.ajax({
                dataType: 'json', method: 'post', url: '/api/' + window.tid + '/waypoint/add',
                contentType: "application/json", data: JSON.stringify({
                    x: atX, y: atY,
                }),
            }).then((result) => {
                if (result.err) {
                    console.error(result);
                    debugger;
                    throw Error(result.err);
                }
                // created wp should be passed over watching ws as notification

            }, (err) => {
                console.error('backend error', err);
            });

        } else if ($('#place_truck').prop('checked')) {

            // call backend to add the wp
            $.ajax({
                dataType: 'json', method: 'post', url: '/api/' + window.tid + '/truck/add',
                contentType: "application/json", data: JSON.stringify({
                    x: atX, y: atY,
                }),
            }).then((result) => {
                if (result.err) {
                    console.error(result);
                    debugger;
                    throw Error(result.err);
                }
                // created wp should be passed over watching ws as notification

            }, (err) => {
                console.error('backend error', err);
            });

        }
    }

})();
