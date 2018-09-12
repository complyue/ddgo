package svcs

import (
	"encoding/json"
	"fmt"
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

var svcPools = make(map[string]*svcpool.Consumer)
var svcMu sync.Mutex

func GetService(serviceKey, wireKey, session string) (svcConn *hbi.TCPConn, err error) {
	svcMu.Lock()
	defer svcMu.Unlock()

	var ok bool

	fullWireKey := fmt.Sprintf("%s@%s", serviceKey, wireKey)
	var consumer *svcpool.Consumer
	consumer, ok = svcPools[fullWireKey]
	if ok && consumer.Pool.Cancelled() {
		delete(svcPools, fullWireKey)
		ok = false
	}
	if !ok {
		var cfg ServiceConfig
		cfg, err = GetServiceConfig(serviceKey)
		if err != nil {
			return
		}
		consumer, err = svcpool.NewConsumer(hbi.NewHoContext, cfg.Addr())
		if err != nil {
			return
		}
		svcPools[fullWireKey] = consumer
	}

	svcConn, err = consumer.GetService(session, true)
	return
}
