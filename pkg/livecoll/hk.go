package livecoll

import (
	"sync"

	"github.com/complyue/ddgo/pkg/isoevt"
	"github.com/complyue/hbigo/pkg/errors"
)

type Member interface {
	GetID() interface{}
}

type HouseKeeper interface {
	Load(fullList []Member)

	Read(id interface{}) (Member, bool)

	Created(mo Member)
	Updated(mo Member)
	Deleted(id interface{})

	Publisher
}

func NewHouseKeeper() HouseKeeper {
	return &houseKeeper{
		ccn:     0,
		members: nil, // only store members if Load() ever called
		ccES:    isoevt.NewStream(),
	}
}

type houseKeeper struct {
	ccn     int // collection change number
	members map[interface{}]Member

	mu sync.RWMutex // collection change mutex

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

func (hk *houseKeeper) Read(id interface{}) (Member, bool) {
	if hk.members == nil {
		panic("Not a loaded collection.")
	}

	hk.mu.RLock()
	defer hk.mu.RUnlock()

	mbyid, ok := hk.members[id]
	return mbyid, ok
}

func (hk *houseKeeper) Created(mo Member) {
	if hk.members != nil {
		func() {
			hk.mu.Lock()
			defer hk.mu.Unlock()

			id := mo.GetID()
			hk.members[id] = mo
		}()
	}

	hk.ccn++

	{
		hk.ccES.Post(CreatedEvent{hk.ccn, mo})
	}
}

func (hk *houseKeeper) Updated(mo Member) {
	if hk.members != nil {
		func() {
			hk.mu.Lock()
			defer hk.mu.Unlock()

			id := mo.GetID()
			hk.members[id] = mo
		}()
	}

	hk.ccn++

	{
		hk.ccES.Post(UpdatedEvent{hk.ccn, mo})
	}
}

func (hk *houseKeeper) Deleted(id interface{}) {
	if hk.members != nil {
		func() {
			hk.mu.Lock()
			defer hk.mu.Unlock()

			if _, ok := hk.members[id]; !ok {
				panic(errors.Errorf("Removing non member id %+v", id))
			}
		}()
	}

	hk.ccn++

	{
		hk.ccES.Post(DeletedEvent{hk.ccn, id})
	}
}

func (hk *houseKeeper) FetchAll() (ccn int, members []Member) {
	if hk.members == nil {
		panic("Not a loaded collection.")
	}

	hk.mu.RLock()
	defer hk.mu.RUnlock()

	members = make([]Member, 0, len(hk.members))
	for _, cmo := range hk.members {
		members = append(members, cmo)
	}
	ccn = hk.ccn
	return
}

func (hk *houseKeeper) Subscribe(subr Subscriber) {
	Dispatch(hk.ccES, subr, func() bool {
		// fire Epoch event upon watching started
		subr.Epoch(hk.ccn)
		return false
	})
}
