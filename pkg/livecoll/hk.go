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
	hk.mu.RLock()
	defer hk.mu.RUnlock()

	mbyid, ok := hk.members[id]
	return mbyid, ok
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
		hk.ccES.Post(UpdateEvent{hk.ccn, mo})
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
