package main

import (
	"flag"
	"net/http"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique/example/chatterbox/server"
)

var addr = flag.String("addr", ":6024", "websocket address")

func main() {
	flag.Parse()
	cb := server.New()

	http.HandleFunc("/", cb.Handler)

	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		glog.Fatal("ListenAndServe: ", err)
	}
}
