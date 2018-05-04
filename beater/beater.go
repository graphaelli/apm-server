package beater

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/elastic/apm-agent-go"
	"github.com/elastic/apm-agent-go/contrib/apmprometheus"
	"github.com/elastic/apm-agent-go/transport"
	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

type beater struct {
	config  *Config
	mutex   sync.Mutex // guards server and stopped
	server  *http.Server
	stopped bool
	logger  *logp.Logger
	tracer  *elasticapm.Tracer
}

// Creates beater
func New(b *beat.Beat, ucfg *common.Config) (beat.Beater, error) {
	beaterConfig := defaultConfig(b.Info.Version)
	if err := ucfg.Unpack(beaterConfig); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}
	if beaterConfig.Frontend.isEnabled() {
		if _, err := regexp.Compile(beaterConfig.Frontend.LibraryPattern); err != nil {
			return nil, errors.New(fmt.Sprintf("Invalid regex for `library_pattern`: %v", err.Error()))
		}
		if _, err := regexp.Compile(beaterConfig.Frontend.ExcludeFromGrouping); err != nil {
			return nil, errors.New(fmt.Sprintf("Invalid regex for `exclude_from_grouping`: %v", err.Error()))
		}
		if b.Config != nil && b.Config.Output.Name() == "elasticsearch" {
			beaterConfig.setElasticsearch(b.Config.Output.Config())
		}
	}
	bt := &beater{
		config:  beaterConfig,
		stopped: false,
		logger:  logp.NewLogger("beater"),
	}
	return bt, nil
}

// parseListener extracts the network and path for a configured host address
// all paths are tcp unix:/path/to.sock
func parseListener(host string) (string, string) {
	if parsed, err := url.Parse(host); err == nil && parsed.Scheme == "unix" {
		return parsed.Scheme, parsed.Path
	}
	return "tcp", host
}

// listen starts the listener for bt.config.Host
// bt.config.Host may be mutated by this function in case the resolved listening address does not match the
// configured bt.config.Host value.
// This should only be called once, from Run.
func (bt *beater) listen() (net.Listener, error) {
	network, path := parseListener(bt.config.Host)
	if network == "tcp" {
		if _, _, err := net.SplitHostPort(path); err != nil {
			// tack on a port if SplitHostPort fails on what should be a tcp network address
			// if there were already too many colons, one more won't hurt
			path = net.JoinHostPort(path, defaultPort)
		}
	}
	lis, err := net.Listen(network, path)
	if err != nil {
		return nil, err
	}
	// in case host is :0 or similar
	if network == "tcp" {
		addr := lis.Addr().(*net.TCPAddr).String()
		if bt.config.Host != addr {
			bt.logger.Infof("host resolved from %s to %s", bt.config.Host, addr)
			bt.config.Host = addr
		}
	}
	return lis, err
}

func (bt *beater) Run(b *beat.Beat) error {
	pub, err := newPublisher(b.Publisher, bt.config.ConcurrentRequests, bt.config.ShutdownTimeout)
	if err != nil {
		return err
	}
	defer pub.Stop()

	lis, err := bt.listen()
	if err != nil {
		bt.logger.Error("failed to listen:", err)
		return nil
	}

	network, _ := parseListener(bt.config.Host)
	if bt.config.SelfInstrument && network == "tcp" {
		go func() {
			protocol := "http://"
			if bt.config.SSL.isEnabled() {
				protocol = "https://"
			}
			if os.Getenv("ELASTIC_APM_SERVER_URL") == "" {
				os.Setenv("ELASTIC_APM_SERVER_URL", protocol+lis.Addr().String())
				// arbitrary wait for server start up
				time.Sleep(time.Second)
				transport.InitDefault()
			}
			bt.tracer, _ = elasticapm.NewTracer("apm-server", "6.x")
			bt.tracer.AddMetricsGatherer(apmprometheus.Wrap(prometheus.DefaultGatherer))
			bt.tracer.SendMetrics(nil)
		}()
	}

	go notifyListening(bt.config, pub.client.Publish)
	bt.mutex.Lock()
	if bt.stopped {
		defer bt.mutex.Unlock()
		return nil
	}

	bt.server = newServer(bt.config, pub.Send)
	bt.mutex.Unlock()
	err = run(bt.server, lis, bt.config)
	if err == http.ErrServerClosed {
		bt.logger.Infof("Listener stopped: %s", err.Error())
		return nil
	}
	return err
}

// Graceful shutdown
func (bt *beater) Stop() {
	bt.logger.Infof("stopping apm-server... waiting maximum of %v seconds for queues to drain",
		bt.config.ShutdownTimeout.Seconds())
	bt.mutex.Lock()
	if bt.server != nil {
		stop(bt.server)
	}
	// race!
	if bt.tracer != nil {
		bt.tracer.Close()
	}
	bt.stopped = true
	bt.mutex.Unlock()
}
