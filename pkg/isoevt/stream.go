// isolated event dispatching
package isoevt

import (
	"log"
	"runtime"
	"sync"
)

func NewStream() *EventStream {
	return &EventStream{
		cnd: sync.NewCond(new(sync.Mutex)),
	}
}

type EventStream struct {
	cnd  *sync.Cond
	tail *evtNode
}

func (es *EventStream) Post(evt interface{}) {
	newTail := &evtNode{
		evt: evt,
	}
	es.cnd.L.Lock()
	defer es.cnd.L.Unlock()
	if es.tail != nil {
		es.tail.next = newTail
	}
	es.tail = newTail
	es.cnd.Broadcast()
}

// unless the event stream is guaranteed to be ever received through a channel,
// callback is better than channel here. if a send channel given here,
// it'll be unclear when should the channel be closed, while it's always the
// sender's responsibility to close a channel, at most another context's `Done()`
// channel can be passed along to signal cancellation, but that's not flexible.
// and if the receiving goroutine crashed anyhow, very old tail pointer will left
// unprogressive, essentially keep useless event records from being garbage collected.
// at the same time, a callback can choose (select) to relay each event to a channel,
// and keep an eye on one or more context's `Done()` signal, in addition to
// performing various versatile checks & operations. either it returns true or panic,
// the watching goro will stop, to have everything around it released.
func (es *EventStream) Watch(
	evtCallback func(evt interface{}) (stop bool),
	watchingCallback func() (stop bool),
) {
	go func() {
		var knownTail, nextEvt *evtNode

		// wait until got non-nil tail
		es.cnd.L.Lock()
		knownTail = es.tail
		if knownTail != nil {
			// don't distribute present tail event, it's considered obsoleted
			nextEvt = knownTail.next
		} else {
			// should distribute the 1st event appeared after watching started
			for ; nextEvt == nil; nextEvt = es.tail {
				es.cnd.Wait()
			}
		}
		es.cnd.L.Unlock()

		if watchingCallback != nil {
			// signal watching started.
			func() {
				defer func() {
					if err := recover(); err != nil {
						// the watching cb opt to stop watching by panic
						log.Printf("Event watching callback error: %+v\n", err)
						runtime.Goexit()
					}
				}()
				if stop := watchingCallback(); stop {
					// watching cb opt to stop watching
					runtime.Goexit()
				}
			}()
		}

		for { // continue dispatching until finished or failed
			for nextEvt != nil { // loop through cached list without sync
				func() {
					defer func() { // catch watcher failure and stop
						if err := recover(); err != nil {
							// the watcher cb opt to stop watching by panic
							log.Printf("Event watcher error: %+v\n", err)
							runtime.Goexit()
						}
					}()
					// the watcher cb can panic to stop watching, process crashing is hereby prevented for it
					if stop := evtCallback(nextEvt.evt); stop {
						// the watcher cb opt to stop watching by return value
						runtime.Goexit()
					}
				}()
				// `nextEvt.next` may be cached value, but is always valid, even without sync.
				// atomic pointer loading required here though, but not a problem until more
				// architectures beyond x64 to be supported.
				knownTail, nextEvt = nextEvt, nextEvt.next
			}
			es.cnd.L.Lock()
			nextEvt = knownTail.next // read again after sync-ed
			if nextEvt == nil {
				es.cnd.Wait() // really need to wait
			}
			es.cnd.L.Unlock()
		}
	}()
}

type evtNode struct {
	evt interface{}

	next *evtNode
}
