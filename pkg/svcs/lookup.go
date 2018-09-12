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
	Url      string
	Host     string
	Port     int
	Parallel int
	Size     int
	Hot      int
	Timeout  string
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

func GetRoutesService(tunnel string, session string) (svcConn *hbi.TCPConn, err error) {
	svcConn, err = getService("routes", routes.NewConsumerContext, tunnel, session)
	return
}

var svcPools = make(map[string]*svcpool.Consumer)
var svcMu sync.Mutex

func getService(
	serviceKey string, ctxFact func() hbi.HoContext,
	tunnel string, session string,
) (svcConn *hbi.TCPConn, err error) {
	svcMu.Lock()
	defer svcMu.Unlock()

	var ok bool

	var consumer *svcpool.Consumer
	consumer, ok = svcPools[serviceKey]
	if !ok {
		var cfg ServiceConfig
		cfg, err = GetServiceConfig(serviceKey)
		if err != nil {
			return
		}
		consumer, err = svcpool.NewConsumer(cfg.Addr(), nil)
		if err != nil {
			return
		}
		svcPools[serviceKey] = consumer
	}

	svcConn, err = consumer.GetService(ctxFact, tunnel, session, true)
	return
}
