// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package beater

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmhttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"

	"github.com/elastic/apm-server/model"
	"github.com/elastic/apm-server/publish"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/outputs"
	"github.com/elastic/beats/libbeat/version"
)

type grpcServer struct{}

func (s *grpcServer) SendEvents(ctx context.Context, in *model.Metadata) (*model.EventsResponse, error) {
	logp.NewLogger("server").Info("in send events")
	return nil, errors.New("wtf")
	//return &model.EventsResponse{}, nil
}

func newServer(config *Config, tracer *apm.Tracer, report publish.Reporter) *http.Server {
	s := grpc.NewServer()
	model.RegisterApmServer(s, &grpcServer{})

	oldMux := apmhttp.Wrap(newMuxer(config, report),
		apmhttp.WithServerRequestIgnorer(doNotTrace),
		apmhttp.WithTracer(tracer),
	)

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 {
			logp.NewLogger("server").Info("grpc!")
			s.ServeHTTP(w, r)
		} else {
			oldMux.ServeHTTP(w, r)
		}
	})

	return &http.Server{
		Addr:           config.Host,
		Handler:        h2c.NewHandler(mux, &http2.Server{}),
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		MaxHeaderBytes: config.MaxHeaderSize,
	}
}

func doNotTrace(req *http.Request) bool {
	if req.RemoteAddr == "pipe" {
		// Don't trace requests coming from self,
		// or we will go into a continuous cycle.
		return true
	}
	if req.URL.Path == rootURL {
		// Don't trace root url (healthcheck) requests.
		return true
	}
	return false
}

func run(server *http.Server, lis net.Listener, config *Config) error {
	logger := logp.NewLogger("server")
	logger.Infof("Starting apm-server [%s built %s]. Hit CTRL-C to stop it.", version.Commit(), version.BuildTime())
	logger.Infof("Listening on: %s", server.Addr)
	switch config.RumConfig.isEnabled() {
	case true:
		logger.Info("RUM endpoints enabled!")
	case false:
		logger.Info("RUM endpoints disabled")
	}

	if config.MaxConnections > 0 {
		lis = netutil.LimitListener(lis, config.MaxConnections)
		logger.Infof("connections limit set to: %d", config.MaxConnections)
	}

	ssl := config.SSL
	if ssl.isEnabled() {
		cert, err := outputs.LoadCertificate(&config.SSL.Certificate)
		if err != nil {
			return err
		}
		server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{*cert}}
		return server.ServeTLS(lis, "", "")
	}
	if config.SecretToken != "" {
		logger.Warn("Secret token is set, but SSL is not enabled.")
	}
	return server.Serve(lis)
}

func stop(server *http.Server) {
	logger := logp.NewLogger("server")
	err := server.Shutdown(context.Background())
	if err != nil {
		logger.Error(err.Error())
		err = server.Close()
		if err != nil {
			logger.Error(err.Error())
		}
	}
}
