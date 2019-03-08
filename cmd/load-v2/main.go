package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func addNewline(p []byte) []byte {
	return append(p, '\n')
}

var (
	metadata    = addNewline([]byte(`{"metadata":{"process":{"pid":1234,"title":"node"},"service":{"agent":{"name":"0.0.1","version":"go-notgrpc"},"name":"service1"}}}`))
	transaction = addNewline([]byte(`{"transaction":{"duration":5,"id":"5e842a69c70cadf3","name":"t1","sampled":true,"span_count":{"started":1},"timestamp":1496170407154000,"trace_id":"e02d9ddfa7d108ad4c99a978fb6d49a0","type":"notviagrpc"}}`))
	span        = addNewline([]byte(`{"span":{"duration":2,"id":"spanid","name":"s1","parent":"5e842a69c70cadf3","parent_id":"5e842a69c70cadf3","timestamp":1496170407154000,"trace_id":"e02d9ddfa7d108ad4c99a978fb6d49a0","transaction_id":"5e842a69c70cadf3","type":"notviagrpc"}}`))
)

func main() {
	transactionCount := flag.Int("t", 10, "transaction count")
	spanCount := flag.Int("s", 5, "transaction count")
	disableCompression := flag.Bool("disable-compression", false, "")
	apmServerHostPort := flag.String("server", "127.0.0.1:8200", "apm server host:port")
	flag.Parse()

	pr, pw := io.Pipe()
	var w io.WriteCloser = pw
	if !*disableCompression {
		w = gzip.NewWriter(pw)
	}

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1,
			DisableCompression:  *disableCompression,
		},
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/intake/v2/events", *apmServerHostPort), pr)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-ndjson")
	if !*disableCompression {
		req.Header.Add("Content-Encoding", "gzip")
	}

	start := time.Now()
	defer func() { fmt.Println("elapsed: ", time.Now().Sub(start).Seconds()) }()
	go func() {
		if _, err = w.Write(metadata); err != nil {
			log.Fatal(err)
		}
		for t := 0; t < *transactionCount; t++ {
			if _, err = w.Write(transaction); err != nil {
				log.Fatal(err)
			}
			for t := 0; t < *spanCount; t++ {
				if _, err = w.Write(span); err != nil {
					log.Fatal(err)
				}
			}
		}
		w.Close()
		pw.Close()
	}()
	rsp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	rsp.Body.Close()
	fmt.Println(rsp.StatusCode)
}
