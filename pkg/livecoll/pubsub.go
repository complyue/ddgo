package livecoll

import (
	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/golang/glog"
)

// Subscriber watches change events from a specific live business object collection.
// Every `Subscribe()` call will create a dedicated goroutine to dispatch boc changes
// to the specified subscriber, normally a subscriber instance should only be passed
// once to `Subscribe()`, so all its event methods are guarranteed to be thread safe.
type Subscriber interface {
	// Subscribed is a subscriber-local event that'll be dispatched at most once (no
	// event will be dispatched if the subscription failed or cancelled by a watching
	// callback), before any other event get dispatched to the subscriber.
	Subscribed() (stop bool)

	// Epoch is a wire-bound event, get dispatched to all subscribers over a wire, when
	// the wire is connected. It is most useful to react to wire reconnection during the
	// subscription course.
	Epoch(ccn int) (stop bool)

	// MemberCreated occurs after the specified business object get created.
	MemberCreated(ccn int, eo Member) (stop bool)

	// MemberUpdated occurs after the specified business object get updated.
	MemberUpdated(ccn int, eo Member) (stop bool)

	// MemberDeleted occurs after the specified business object get deleted.
	MemberDeleted(ccn int, id interface{}) (stop bool)
}

// Publisher .
type Publisher interface {
	Subscribe(subr Subscriber)

	FetchAll() (ccn int, members []Member)
}

// ChgDistance measures the distance between 2 collection change numbers.
func ChgDistance(toCCN, fromCCN int) int {
	// CCN is monotonic incremental, but may overflow/wraparound after many changes occurred
	if (toCCN >= 0 && fromCCN >= 0) || (toCCN < 0 && fromCCN < 0) {
		// same sign, assuming NO overflow/wraparound has occurred
		return toCCN - fromCCN
	}
	// different sign, assuming overflow/wraparound has occurred
	// todo figure out logic to handle this case
	panic("int overflow of collection change number has to be handled!")
}

// Dispatch .
func Dispatch(ccES *isoevt.EventStream, subr Subscriber, watchingCallback func() bool) {
	ccES.Watch(func(evt interface{}) bool {
		switch evo := evt.(type) {
		case EpochEvent:
			return subr.Epoch(evo.CCN)
		case CreatedEvent:
			return subr.MemberCreated(evo.CCN, evo.EO)
		case UpdatedEvent:
			return subr.MemberUpdated(evo.CCN, evo.EO)
		case DeletedEvent:
			return subr.MemberDeleted(evo.CCN, evo.ID)
		default:
			panic(errors.Errorf("Event of type %T ?!", evt))
		}
	}, func() (stop bool) {
		defer func() {
			if e := recover(); e != nil {
				glog.Warningf("Subscription cancelled due to error: %+v", e)
				stop = true
			}
		}()
		if watchingCallback != nil && watchingCallback() {
			// watching cb opt'ed to stop by returning false
			stop = true
		}
		if !stop { // if watching cb decided to stop watching, do not invoke Subscribed event
			stop = subr.Subscribed()
		}
		return
	})
}

// EpochEvent .
type EpochEvent struct {
	CCN int
}

// CreatedEvent .
type CreatedEvent struct {
	CCN int
	EO  Member
}

// UpdatedEvent .
type UpdatedEvent struct {
	CCN int
	EO  Member
}

// DeletedEvent .
type DeletedEvent struct {
	CCN int
	ID  interface{}
}
