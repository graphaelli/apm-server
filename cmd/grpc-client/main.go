package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	"github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/grpc"

	"github.com/elastic/apm-server/model"
)

func main() {
	conn, err := grpc.Dial("127.0.0.1:8200", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	c := model.NewApmClient(conn)
	stream, err := c.Insert(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	tTime := time.Now().UnixNano()

	events := []model.Event{
		{
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
		},
		{
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
		},
		{
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
		},
	}

	for _, event := range events {
		switch ev := event.GetApm().(type) {
		case *model.Event_Error:
			if err := ev.Error.Validate(); err != nil {
				log.Println("submitting invalid error payload:", err)
			}
		case *model.Event_Span:
			if err := ev.Span.Validate(); err != nil {
				log.Println("submitting invalid span payload:", err)
			}
		case *model.Event_Transaction:
			if err := ev.Transaction.Validate(); err != nil {
				log.Println("submitting invalid transaction payload:", err)
			}
		}
		if err := stream.Send(&event); err != nil {
			log.Fatal(err)
		}
		log.Println("sent", &event)
	}
}
