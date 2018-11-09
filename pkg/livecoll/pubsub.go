package livecoll

type Subscriber interface {
	Epoch(ccn int) bool

	MemberCreated(ccn int, eo Member) bool
	MemberUpdated(ccn int, eo Member) bool
	MemberDeleted(ccn int, eo Member) bool
}

type Publisher interface {
	Subscribe(subr Subscriber)

	FetchAll() (ccn int, members []Member)
}
