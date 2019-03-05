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
	rsp, err := c.SendEvents(context.TODO(), &model.Metadata{})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(rsp)
}
