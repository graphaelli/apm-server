package main

import (
	"context"
	"log"

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

	events := []model.Event{
		{
			Apm: &model.Event_Metadata{
				Metadata: &model.Metadata{
					Service: &model.Service{
						Name: "service1",
						Agent: &model.Agent{
							Name:    "go-grpc",
							Version: "0.0.1",
						},
					},
				},
			},
		},
		{
			Apm: &model.Event_Transaction{
				Transaction: &model.Transaction{
					Id:   "txid",
					Name: "t1",
				},
			},
		},
		{
			Apm: &model.Event_Span{
				Span: &model.Span{
					Id:   "spanid",
					Name: "s1",
				},
			},
		},
	}

	for _, event := range events {
		if err := stream.Send(&event); err != nil {
			log.Fatal(err)
		}
		log.Println("sent", &event)
	}
}
