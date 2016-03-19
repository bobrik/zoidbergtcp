package zoidbergtcp

import (
	"io"
	"log"
	"math/rand"
	"net"
	"reflect"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/bobrik/zoidberg/application"
	"github.com/bobrik/zoidberg/state"
)

// copySize defines the size of a chunk that is used when copying data,
// it also defines minimum granularity for stats
const copySize = 4096

// proxy represents a tcp proxy with upstreams
type proxy struct {
	um sync.Mutex
	u  upstreams
	l  net.Listener
	sm sync.Mutex
	s  map[string]*upstreamStats
}

// newProxy creates a new tcp proxy
func newProxy(listen string) (*proxy, error) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}

	return &proxy{
		l:  listener,
		um: sync.Mutex{},
		u:  []upstream{},
		sm: sync.Mutex{},
		s:  map[string]*upstreamStats{},
	}, nil
}

// stats returns proxyStats with stats for upstreams,
// that were ever used. If an upstream was never used,
// there is no guarantee it is present in the result
func (p *proxy) stats() proxyStats {
	u := map[string]upstreamStats{}

	p.sm.Lock()
	for k, v := range p.s {
		u[k] = *v
	}
	p.sm.Unlock()

	return proxyStats{
		Upstreams: u,
	}
}

// upstreamStats return upstreamStats for specific upstream,
// it allocates object lazily if needed
func (p *proxy) upstreamStats(u string) *upstreamStats {
	p.sm.Lock()

	s := &upstreamStats{}
	if es, ok := p.s[u]; !ok {
		p.s[u] = s
	} else {
		s = es
	}
	p.sm.Unlock()

	return s
}

// setState sets state for the proxy based on servers and their versions
func (p *proxy) setState(port int, servers []application.Server, versions state.Versions) {
	p.um.Lock()

	u := upstreams{}

	for _, s := range servers {
		if len(s.Ports) < port {
			log.Printf("requesed missing port %d from %s\n", port, s)
			continue
		}

		w := 1
		if len(versions) > 0 {
			w = versions[s.Version].Weight
		}

		if w == 0 {
			continue
		}

		u = append(u, upstream{
			host:   s.Host,
			port:   s.Ports[port],
			weight: w,
		})
	}

	sort.Sort(u)

	if !reflect.DeepEqual(u, p.u) {
		p.u = u
		log.Printf("updated upstreams for %s: %s\n", p.l.Addr(), u)
	}

	p.um.Unlock()
}

// start starts main proxy loop
func (p *proxy) start() {
	addr := p.l.Addr()

	for {
		c, err := p.l.Accept()
		if err != nil {
			log.Printf("error on accept for %s: %s", addr, err)
			continue
		}

		go p.serve(c.(*net.TCPConn))
	}
}

// serve serves a single accepted connection
func (p *proxy) serve(c *net.TCPConn) {
	defer func() {
		_ = c.Close()
	}()

	addr := p.l.Addr()

	p.um.Lock()
	upstreams := make([]upstream, len(p.u))
	copy(upstreams, p.u)
	p.um.Unlock()

	// TODO: proper iterator accounting weights and current # of connections
	for i := range upstreams {
		j := rand.Intn(i + 1)
		upstreams[i], upstreams[j] = upstreams[j], upstreams[i]
	}

	for _, u := range upstreams {
		s := p.upstreamStats(u.addr())

		log.Printf("connecting to %s for %s..\n", u, addr)
		r, err := net.Dial("tcp", u.addr())
		if err != nil {
			p.sm.Lock()
			s.Errors++
			p.sm.Unlock()
			log.Printf("error connecting for %s to %s: %s", addr, u.addr(), err)
			continue
		}

		p.sm.Lock()
		s.Connected++
		s.Connections++
		p.sm.Unlock()

		defer func() {
			p.sm.Lock()
			s.Connected--
			p.sm.Unlock()
		}()

		p.proxyLoop(c, r.(*net.TCPConn), s)

		break
	}
}

// TODO: replace with some 3rd party package
// https://github.com/docker/docker/blob/18c7c67308bd4a24a41028e63c2603bb74eac85e/pkg/proxy/tcp_proxy.go#L34
func (p *proxy) proxyLoop(client, backend *net.TCPConn, s *upstreamStats) {
	started := time.Now()

	event := make(chan struct{})
	var broker = func(to, from *net.TCPConn, counter *uint64) {
		for {
			n, err := io.CopyN(to, from, copySize)
			p.sm.Lock()
			*counter += uint64(n)
			p.sm.Unlock()
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

	go broker(client, backend, &s.Out)
	go broker(backend, client, &s.In)

	for i := 0; i < 2; i++ {
		<-event
	}

	_ = client.Close()
	_ = backend.Close()

	ca := client.RemoteAddr()
	ba := backend.RemoteAddr()

	elapsed := time.Now().Sub(started)

	log.Printf(
		"transferred %d bytes between %s and %s (%d in, %d out) in %v",
		s.In+s.Out, ca, ba, s.In, s.Out, elapsed,
	)
}
