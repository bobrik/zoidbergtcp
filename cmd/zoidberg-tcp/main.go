package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bobrik/zoidbergtcp"
)

func main() {
	listen := flag.String("listen", fmt.Sprintf("%s:%s", os.Getenv("HOST"), os.Getenv("PORT")), "listen address")
	flag.Parse()

	manager := zoidbergtcp.NewManager()

	err := http.ListenAndServe(*listen, manager.ServeMux())
	if err != nil {
		log.Fatal(err)
	}
}
