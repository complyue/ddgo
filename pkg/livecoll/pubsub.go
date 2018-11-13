package livecoll

import (
	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/hbigo/pkg/errors"
)

type Subscriber interface {
	Epoch(ccn int) (stop bool)

	MemberCreated(ccn int, eo Member) (stop bool)
	MemberUpdated(ccn int, eo Member) (stop bool)
	MemberDeleted(ccn int, eo Member) (stop bool)
}

type Publisher interface {
	Subscribe(subr Subscriber)

	FetchAll() (ccn int, members []Member)
}

func Dispatch(ccES *isoevt.EventStream, subr Subscriber, watchingCallback func() bool) {
	ccES.Watch(func(evt interface{}) bool {
		switch evo := evt.(type) {
		case EpochEvent:
			return subr.Epoch(evo.CCN)
		case CreateEvent:
			return subr.MemberCreated(evo.CCN, evo.EO)
		case UpdateEvent:
			return subr.MemberUpdated(evo.CCN, evo.EO)
		case DeleteEvent:
			return subr.MemberDeleted(evo.CCN, evo.EO)
		default:
			panic(errors.Errorf("Event of type %T ?!", evt))
		}
	}, watchingCallback)
}

type EpochEvent struct {
	CCN int
}

type CreateEvent struct {
	CCN int
	EO  Member
}

type UpdateEvent struct {
	CCN int
	EO  Member
}

type DeleteEvent struct {
	CCN int
	EO  Member
}
