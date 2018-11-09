package livecoll

import (
	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/hbigo/pkg/errors"
	"sync"
)

type HouseKeeper interface {
	Load(fullList []Member)

	Create(mo Member)
	Update(mo Member)
	Delete(mo Member)

	Publisher
}

func NewHouseKeeper() HouseKeeper {
	return &houseKeeper{
		ccn:     0,
		members: make(map[interface{}]Member),
		ccES:    isoevt.NewStream(),
	}
}

type houseKeeper struct {
	ccn     int // collection change number
	members map[interface{}]Member

	mu sync.Mutex // collection change mutex

	ccES *isoevt.EventStream // collection change event stream
}

func (hk *houseKeeper) Load(fullList []Member) {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	hk.members = make(map[interface{}]Member)
	for _, mo := range fullList {
		hk.members[mo.GetID()] = mo
	}
	hk.ccn = 0

	{
		hk.ccES.Post(EpochEvent{hk.ccn})
	}
}

func (hk *houseKeeper) Create(mo Member) {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	id := mo.GetID()
	hk.members[id] = mo
	hk.ccn++

	{
		hk.ccES.Post(CreateEvent{hk.ccn, mo})
	}
}

func (hk *houseKeeper) Update(mo Member) {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	id := mo.GetID()
	hk.members[id] = mo
	hk.ccn++

	{
		hk.ccES.Post(CreateEvent{hk.ccn, mo})
	}
}

func (hk *houseKeeper) Delete(mo Member) {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	id := mo.GetID()
	if cmo, ok := hk.members[id]; !ok || cmo != mo {
		panic(errors.Errorf("Removing non member %T - %+v", mo, mo))
	}
	hk.ccn++

	{
		hk.ccES.Post(DeleteEvent{hk.ccn, mo})
	}
}

func (hk *houseKeeper) FetchAll() (ccn int, members []Member) {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	members = make([]Member, len(hk.members))
	for _, cmo := range hk.members {
		members = append(members, cmo)
	}
	ccn = hk.ccn
	return
}

func (hk *houseKeeper) Subscribe(subr Subscriber) {
	hk.ccES.Watch(func(evt interface{}) bool {
		switch evo := evt.(type) {
		case EpochEvent:
			return subr.Epoch(evo.ccn)
		case CreateEvent:
			return subr.MemberCreated(evo.ccn, evo.eo)
		case UpdateEvent:
			return subr.MemberUpdated(evo.ccn, evo.eo)
		case DeleteEvent:
			return subr.MemberDeleted(evo.ccn, evo.eo)
		default:
			panic(errors.Errorf("Event of type %T ?!", evt))
		}
	})
}

type EpochEvent struct {
	ccn int
}

type CreateEvent struct {
	ccn int
	eo  Member
}

type UpdateEvent struct {
	ccn int
	eo  Member
}

type DeleteEvent struct {
	ccn int
	eo  Member
}
