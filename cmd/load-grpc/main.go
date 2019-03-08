package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc"

	"github.com/elastic/apm-server/model"
)

var (
	metadata = &model.Event{
		Apm: &model.Event_Metadata{
			Metadata: &model.Metadata{
				Service: &model.Service{
					Name:    "service1",
					Version: "alpha",
					Agent: &model.Service_Agent{
						Name:    "go-grpc",
						Version: "0.0.1",
					},
				},
				Process: &model.Process{
					Pid:   uint32(os.Getpid()),
					Title: filepath.Base(os.Args[0]),
				},
			},
		},
	}

	tTime = time.Now().UnixNano()

	transaction = &model.Event{
		Apm: &model.Event_Transaction{
			Transaction: &model.Transaction{
				Timestamp: &timestamp.Timestamp{
					Seconds: int64(tTime / 1e9),
					Nanos:   int32((tTime % 1e9) * 1000),
				},
				Duration: &duration.Duration{Seconds: 5},
				Id:       "5e842a69c70cadf3",
				Name:     "t1",
				TraceId:  "e02d9ddfa7d108ad4c99a978fb6d49a0",
				Type:     "viagrpc",
				Sampled:  true,
				SpanCount: &model.Transaction_SpanCount{
					Started: 1,
				},
			},
		},
	}

	span = &model.Event{
		Apm: &model.Event_Span{
			Span: &model.Span{
				Duration: &duration.Duration{Seconds: 2},
				Id:       "spanid",
				Name:     "s1",
				ParentId: "5e842a69c70cadf3",
				Timestamp: &timestamp.Timestamp{
					Seconds: int64(tTime/1e9) + 1,
					Nanos:   int32((tTime%1e9)*1000) + 111,
				},
				TransactionId: "5e842a69c70cadf3",
				TraceId:       "e02d9ddfa7d108ad4c99a978fb6d49a0",
				Type:          "viagrpc",
			},
		},
	}
)

func main() {
	transactionCount := flag.Int("t", 10, "transaction count")
	spanCount := flag.Int("s", 5, "transaction count")
	apmServerHostPort := flag.String("server", "127.0.0.1:8200", "apm server host:port")
	flag.Parse()

	conn, err := grpc.Dial(*apmServerHostPort, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	c := model.NewApmClient(conn)
	stream, err := c.Insert(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	start := time.Now()
	defer func() { fmt.Println("elapsed: ", time.Now().Sub(start).Seconds()) }()
	if err = stream.Send(metadata); err != nil {
		log.Fatal(err)
	}
	for t := 0; t < *transactionCount; t++ {
		if err = stream.Send(transaction); err != nil {
			log.Fatal(err)
		}
		for t := 0; t < *spanCount; t++ {
			if err = stream.Send(span); err != nil {
				log.Fatal(err)
			}
		}
	}
	rsp, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatal(err)
	}
	log.Println(rsp.String())
}
