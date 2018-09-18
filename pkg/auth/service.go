package auth

import "github.com/complyue/hbigo"

// construct a service hosting context for serving over HBI wires
func NewServiceContext() hbi.HoContext {
	return &serviceContext{
		HoContext: hbi.NewHoContext(),
	}
}

// implementation details of service context
type serviceContext struct {
	hbi.HoContext
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *serviceContext) TypesToExpose() []interface{} {
	return []interface{}{}
}

// this service method has rpc style, with err-out converted to panic,
// which will induce forceful disconnection
func (ctx *serviceContext) AuthenticateUser(tid, uid, pwd, tkn string) string {
	newTkn, err := AuthenticateUser(tid, uid, pwd, tkn)
	if err != nil {
		panic(err)
	}
	return newTkn
}

// this service method has rpc style, with err-out converted to panic,
// which will induce forceful disconnection
func (ctx *serviceContext) RegisterUser(tid, uid, pwd string) bool {
	ok, err := RegisterUser(tid, uid, pwd)
	if err != nil {
		panic(err)
	}
	return ok
}
