package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bobrik/zoidberg/balancer"
	"github.com/bobrik/zoidbergtcp"
)

func main() {
	listen := flag.String("listen", fmt.Sprintf("%s:%s", os.Getenv("HOST"), os.Getenv("PORT")), "listen address")
	flag.Parse()

	manager := zoidbergtcp.NewManager()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		state := balancer.State{}

		err := json.NewDecoder(r.Body).Decode(&state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager.UpdateState(state)
	})

	mux.HandleFunc("/_health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	err := http.ListenAndServe(*listen, mux)
	if err != nil {
		log.Fatal(err)
	}
}
