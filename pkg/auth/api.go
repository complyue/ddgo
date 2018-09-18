package auth

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/svcs"
	"github.com/complyue/hbigo"
)

func GetAuthService() (*ConsumerAPI, error) {
	if svc, err := svcs.GetService("auth", func() hbi.HoContext {
		api := NewConsumerAPI()
		ctx := api.GetHoCtx()
		ctx.Put("api", api)
		return ctx
	}, // single tunnel, session-less, share a pool of service globally
		"", "", false); err != nil {
		return nil, err
	} else {
		return svc.Hosting.HoCtx().Get("api").(*ConsumerAPI), nil
	}
}

func NewConsumerAPI() *ConsumerAPI {
	return &ConsumerAPI{}
}

type ConsumerAPI struct {
	ctx *consumerContext
}

func (api *ConsumerAPI) AuthenticateUser(tid, uid, pwd, tkn string) (string, error) {
	ctx := api.ctx
	if ctx == nil {
		return AuthenticateUser(tid, uid, pwd, tkn)
	}

	co, err := ctx.PoToPeer().Co()
	if err != nil {
		return "", err
	}
	defer co.Close()
	newTkn, err := co.Get(fmt.Sprintf(`
AuthenticateUser(%#v,%#v,%#v,%#v)
`, tid, uid, pwd, tkn), nil)
	return newTkn.(string), err
}

func (api *ConsumerAPI) RegisterUser(tid, uid, pwd string) (bool, error) {
	ctx := api.ctx
	if ctx == nil {
		return RegisterUser(tid, uid, pwd)
	}

	co, err := ctx.PoToPeer().Co()
	if err != nil {
		return false, err
	}
	defer co.Close()
	ok, err := co.Get(fmt.Sprintf(`
RegisterUser(%#v,%#v,%#v)
`, tid, uid, pwd), nil)
	return ok.(bool), err
}

// once invoked, the returned ctx must be used to establish a HBI connection
// to a remote service.
func (api *ConsumerAPI) GetHoCtx() hbi.HoContext {
	if api.ctx == nil {
		api.ctx = &consumerContext{
			HoContext: hbi.NewHoContext(),
		}
	}
	return api.ctx
}

// implementation details at consumer endpoint for service consuming over HBI wire
type consumerContext struct {
	hbi.HoContext
}

// give types to be exposed, with typed nil pointer values to each
func (ctx *consumerContext) TypesToExpose() []interface{} {
	return []interface{}{}
}
