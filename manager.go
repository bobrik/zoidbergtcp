package zoidbergtcp

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/bobrik/zoidberg/application"
	"github.com/bobrik/zoidberg/balancer"
	"github.com/bobrik/zoidberg/state"
	"github.com/prometheus/client_golang/prometheus"
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

	mux.Handle("/metrics", prometheus.Handler())

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
	listen := app.Meta["listen"]

	if listen == "" {
		log.Printf("app %s does not have listen set in meta", app.Name)
		return
	}

	if proxy, ok := m.proxies[listen]; ok {
		if proxy.app != app.Name {
			log.Printf("app %s to overwrites listen of app %s: %s", app.Name, proxy.app, listen)
		}
		proxy.setState(app.Servers, versions)
		return
	}

	proxy, err := newProxy(app.Name, listen)
	if err != nil {
		log.Printf("error creating proxy for app %s: %s", listen, err)
		return
	}

	proxy.setState(app.Servers, versions)
	go proxy.start()

	m.proxies[listen] = proxy
}
