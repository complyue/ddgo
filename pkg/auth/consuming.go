package auth

import (
	"github.com/complyue/ddgo/pkg/svcs"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"thingswell.com/ids-base/server/routers"
)

func accountCreate(req routers.AccountCreateReq) (ret *routers.AccountRsp, err error) {
	ret = &routers.AccountRsp{}
	err = rpcCall("Account.Create", req, ret)
	return
}

func accountLogin(req routers.LoginReq) (ret *routers.AccountRsp, err error) {
	ret = &routers.AccountRsp{}
	err = rpcCall("Account.Login", req, ret)
	return
}

func accountSetPassword(req routers.SetPswReq) (ret *routers.CommonRsp, err error) {
	ret = &routers.CommonRsp{}
	err = rpcCall("Account.SetPassword", req, ret)
	return
}

func accountRemove(id string) (ret *routers.CommonRsp, err error) {
	ret = &routers.CommonRsp{}
	err = rpcCall("Account.Remove", id, ret)
	return
}

var client *rpc.Client

func ensureConnected() error {
	if client != nil {
		return nil
	}
	cfg, err := svcs.GetServiceConfig("account")
	if err != nil {
		return err
	}
	conn, err := net.Dial("tcp", cfg.Addr())
	if err != nil {
		return err
	}
	client = jsonrpc.NewClient(conn)
	return nil
}

func rpcCall(method string, req interface{}, ret interface{}) error {
	if err := ensureConnected(); err != nil {
		return err
	}

	return client.Call(method, req, ret)
}
