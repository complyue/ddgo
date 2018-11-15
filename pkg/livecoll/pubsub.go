package livecoll

import (
	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/hbigo/pkg/errors"
)

type Subscriber interface {
	Epoch(ccn int) (stop bool)

	MemberCreated(ccn int, eo Member) (stop bool)
	MemberUpdated(ccn int, eo Member) (stop bool)
	MemberDeleted(ccn int, id interface{}) (stop bool)
}

type Publisher interface {
	Subscribe(subr Subscriber)

	FetchAll() (ccn int, members []Member)
}

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
	}, watchingCallback)
}

type EpochEvent struct {
	CCN int
}

type CreatedEvent struct {
	CCN int
	EO  Member
}

type UpdatedEvent struct {
	CCN int
	EO  Member
}

type DeletedEvent struct {
	CCN int
	ID  interface{}
}
