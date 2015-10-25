package zoidbergtcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/bobrik/zoidberg/application"
	"github.com/bobrik/zoidberg/balancer"
	"github.com/bobrik/zoidberg/state"
)

// Manager manages proxies
type Manager struct {
	mutex   sync.Mutex
	proxies map[string]*proxy
}

// NewManager creates new proxy manager
func NewManager() *Manager {
	return &Manager{
		mutex:   sync.Mutex{},
		proxies: map[string]*proxy{},
	}
}

// ServeMux returns a ServeMux object that is used to manage proxies
func (m *Manager) ServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		state := balancer.State{}

		err := json.NewDecoder(r.Body).Decode(&state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		m.UpdateState(state)
	})

	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		p := map[string]proxyStats{}

		m.mutex.Lock()
		for k, v := range m.proxies {
			p[k] = v.stats()
		}
		m.mutex.Unlock()

		s, err := json.Marshal(stats{Servers: p})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-type", "application/json")
		_, _ = w.Write(s)
	})

	mux.HandleFunc("/_health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

// UpdateState updates manager's view of the world
func (m *Manager) UpdateState(s balancer.State) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, app := range s.Apps {
		m.updateAppProxies(app, s.State.Versions[app.Name])
	}
}

// updateAppProxies updates upstreams for running proxies
// and starts new proxies if needed
func (m *Manager) updateAppProxies(app application.App, versions state.Versions) {
	p := 0
	for _, s := range app.Servers {
		if len(s.Ports) > p {
			p = len(s.Ports)
		}
	}

	for i := 0; i < p; i++ {
		if app.Meta[fmt.Sprintf("port_%d_type", i)] != "tcp" {
			log.Printf("app %s port %d: not tcp\n", app.Name, i)
			continue
		}

		l := app.Meta[fmt.Sprintf("port_%d_listen", i)]
		if l == "" {
			log.Printf("app %s port %d: no listen\n", app.Name, i)
			continue
		}

		if proxy, ok := m.proxies[l]; ok {
			proxy.setState(i, app.Servers, versions)
			continue
		}

		proxy, err := newProxy(l)
		if err != nil {
			log.Printf("error creating proxy for %s: %s\n", l, err)
			continue
		}

		proxy.setState(i, app.Servers, versions)
		go proxy.start()

		m.proxies[l] = proxy
	}
}
