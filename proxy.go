package zoidbergtcp

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"reflect"
	"sort"
	"sync"
	"syscall"

	"github.com/bobrik/zoidberg/application"
	"github.com/bobrik/zoidberg/state"
	"github.com/prometheus/client_golang/prometheus"
)

// copySize defines the size of a chunk that is used when copying data,
// it also defines minimum granularity for stats
const copySize = 4096

var (
	bytesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxy_bytes_sent",
			Help: "bytes sent to the clients",
		},
		[]string{"app", "upstream"},
	)

	bytesReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxy_bytes_received",
			Help: "bytes received from the clients",
		},
		[]string{"app", "upstream"},
	)

	connectionsAccepted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxy_connections_accepted",
			Help: "number of client connections accepted",
		},
		[]string{"app"},
	)

	connectedClients = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zoidberg_proxy_connected",
			Help: "number of connected clients",
		},
		[]string{"app"},
	)

	connectionErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxy_connection_errors",
			Help: "number of connection errors to upstreams",
		},
		[]string{"app", "upstream"},
	)

	proxyUpstreams = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zoidberg_proxy_upstreams",
			Help: "number of upstreams per proxy",
		},
		[]string{"app"},
	)

	proxyUpstreamUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxy_upstream_updates",
			Help: "number of upstream updates per proxy",
		},
		[]string{"app"},
	)

	proxiesCreated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxies_created",
			Help: "number of proxies created",
		},
		[]string{"app"},
	)

	proxyCreationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zoidberg_proxy_creation_errors",
			Help: "number of proxies failed on creation",
		},
		[]string{"app"},
	)
)

func init() {
	prometheus.MustRegister(bytesSent)
	prometheus.MustRegister(bytesReceived)
	prometheus.MustRegister(connectionsAccepted)
	prometheus.MustRegister(connectedClients)
	prometheus.MustRegister(connectionErrors)
	prometheus.MustRegister(proxyUpstreams)
	prometheus.MustRegister(proxyUpstreamUpdates)
	prometheus.MustRegister(proxiesCreated)
	prometheus.MustRegister(proxyCreationErrors)
}

// proxy represents a tcp proxy with upstreams
type proxy struct {
	mutex     sync.Mutex
	app       string
	listen    string
	listener  net.Listener
	upstreams Upstreams
	labels    prometheus.Labels
}

// newProxy creates a new tcp proxy
func newProxy(app string, listen string) (*proxy, error) {
	labels := prometheus.Labels{"app": app}

	listener, err := net.Listen("tcp", listen)
	if err != nil {
		proxyCreationErrors.With(labels).Inc()
		return nil, err
	}

	proxiesCreated.With(labels).Inc()

	return &proxy{
		mutex:     sync.Mutex{},
		app:       app,
		listen:    listen,
		listener:  listener,
		upstreams: []Upstream{},
		labels:    labels,
	}, nil
}

// setState sets state for the proxy based on servers and their versions
func (p *proxy) setState(servers []application.Server, versions state.Versions) {
	p.mutex.Lock()

	upstreams := Upstreams{}

	for _, server := range servers {
		weight := 1
		if len(versions) > 0 {
			weight = versions[server.Version].Weight
		}

		if weight == 0 {
			continue
		}

		upstreams = append(upstreams, Upstream{
			host:   server.Host,
			port:   server.Port,
			weight: weight,
		})
	}

	sort.Sort(upstreams)

	if !reflect.DeepEqual(upstreams, p.upstreams) {
		p.upstreams = upstreams
		p.log(fmt.Sprintf("updated upstreams: %s", upstreams))
		proxyUpstreamUpdates.With(p.labels).Inc()
		proxyUpstreams.With(p.labels).Set(float64(len(p.upstreams)))
	}

	p.mutex.Unlock()
}

// start starts main proxy loop
func (p *proxy) start() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			p.log(fmt.Sprintf("error on accepting: %s", err))
			continue
		}

		connectionsAccepted.With(p.labels).Inc()

		go p.serve(client.(*net.TCPConn))
	}
}

// serve serves a single accepted connection
func (p *proxy) serve(client *net.TCPConn) {
	connected := connectedClients.With(p.labels)
	connected.Inc()

	defer func() {
		connected.Dec()
		_ = client.Close()
	}()

	p.mutex.Lock()
	upstreams := make([]Upstream, len(p.upstreams))
	copy(upstreams, p.upstreams)
	p.mutex.Unlock()

	// TODO: proper iterator accounting weights and current # of connections
	for i := range upstreams {
		j := rand.Intn(i + 1)
		upstreams[i], upstreams[j] = upstreams[j], upstreams[i]
	}

	for _, upstream := range upstreams {
		p.log(fmt.Sprintf("connecting from %s to %s", client.RemoteAddr(), upstream))
		backend, err := net.Dial("tcp", upstream.Addr())
		if err != nil {
			p.log(fmt.Sprintf("error connecting from %s to %s: %s", client.RemoteAddr(), upstream.Addr(), err))
			connectionErrors.With(prometheus.Labels{"app": p.app, "upstream": upstream.Addr()}).Inc()
			continue
		}

		p.proxyLoop(client, backend.(*net.TCPConn))

		break
	}
}

// https://github.com/docker/docker/blob/18c7c67308bd4a24a41028e63c2603bb74eac85e/pkg/proxy/tcp_proxy.go#L34
func (p *proxy) proxyLoop(client, backend *net.TCPConn) {
	event := make(chan struct{})
	var broker = func(to, from *net.TCPConn, c prometheus.Counter) {
		for {
			n, err := io.CopyN(to, from, copySize)
			c.Add(float64(n))
			if err != nil {
				// If the socket we are writing to is shutdown with
				// SHUT_WR, forward it to the other end of the pipe:
				if err, ok := err.(*net.OpError); ok && err.Err == syscall.EPIPE {
					_ = from.CloseWrite()
				}

				break
			}
		}

		_ = to.CloseRead()
		event <- struct{}{}
	}

	backendAddr := backend.RemoteAddr().String()
	labels := prometheus.Labels{"app": p.app, "upstream": backendAddr}

	go broker(client, backend, bytesSent.With(labels))
	go broker(backend, client, bytesReceived.With(labels))

	for i := 0; i < 2; i++ {
		<-event
	}

	_ = client.Close()
	_ = backend.Close()

	p.log(fmt.Sprintf("closed connection from %s to %s", client.RemoteAddr(), backend.RemoteAddr()))
}

func (p *proxy) log(msg string) {
	log.Printf("proxy[app=%s, listen=%s]: %s", p.app, p.listen, msg)
}
