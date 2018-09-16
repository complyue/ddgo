package svcs

import (
	"encoding/json"
	"fmt"
	"github.com/complyue/ddgo/pkg/routes"
	"github.com/complyue/hbigo"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/complyue/hbigo/pkg/svcpool"
	"io/ioutil"
	"os"
	"sync"
)

type ServiceConfig struct {
	Http, Https string
	Url         string
	Host        string
	Port        int
	Parallel    int
	Size        int
	Hot         int
	Timeout     string
}

func (cfg ServiceConfig) Addr() string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

var svcConfigs map[string]ServiceConfig

func GetServiceConfig(serviceKey string) (cfg ServiceConfig, err error) {
	var ok bool
	if svcConfigs == nil {
		var servicesEtc []byte
		servicesEtc, err = ioutil.ReadFile("etc/services.json")
		if err != nil {
			cwd, _ := os.Getwd()
			panic(errors.Wrapf(err, "Can NOT read etc/services.json`, [%s] may not be the right directory ?\n", cwd))
		}
		err = json.Unmarshal(servicesEtc, &svcConfigs)
		if err != nil {
			return
		}
		svcConfigs = make(map[string]ServiceConfig)
		err = json.Unmarshal(servicesEtc, &svcConfigs)
		if err != nil {
			panic(errors.Wrap(err, "Failed parsing services.json"))
		}
	}
	cfg, ok = svcConfigs[serviceKey]
	if !ok {
		err = errors.New(fmt.Sprintf("No such service: %s", serviceKey))
	}
	return
}

/*
	use tid as session for tenant isolation,
	and tunnel can further be specified to isolate per tenant or per other means
*/
func GetRoutesService(tunnel string, session string) (*routes.ConsumerAPI, error) {
	if svc, err := getService("routes", func() hbi.HoContext {
		api := routes.NewConsumerAPI()
		ctx := api.GetHoCtx()
		ctx.Put("api", api)
		return ctx
	}, tunnel, session); err != nil {
		return nil, err
	} else {
		return svc.Hosting.HoCtx().Get("api").(*routes.ConsumerAPI), nil
	}
}

func getService(
	serviceKey string, ctxFact func() hbi.HoContext,
	tunnel string, session string,
) (svcConn *hbi.TCPConn, err error) {
	consumer, err := getServicePool(serviceKey)
	if err != nil {
		return nil, err
	}
	svcConn, err = consumer.GetService(ctxFact, tunnel, session, true)
	return
}

var svcPools = make(map[string]*svcpool.Consumer)
var muPools sync.Mutex

func getServicePool(serviceKey string) (*svcpool.Consumer, error) {
	muPools.Lock()
	defer muPools.Unlock()

	if consumer, ok := svcPools[serviceKey]; ok {
		return consumer, nil
	}

	cfg, err := GetServiceConfig(serviceKey)
	if err != nil {
		return nil, err
	}
	consumer, err := svcpool.NewConsumer(cfg.Addr(), nil)
	if err != nil {
		return nil, err
	}
	svcPools[serviceKey] = consumer
	return consumer, nil
}
