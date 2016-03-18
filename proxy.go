package zoidbergtcp

import (
	"io"
	"log"
	"math/rand"
	"net"
	"reflect"
	"sort"
	"sync"

	"github.com/bobrik/zoidberg/application"
	"github.com/bobrik/zoidberg/state"
)

// copySize defines the size of a chunk that is used when copying data,
// it also defines minimum granularity for stats
const copySize = 1024

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

		go p.serve(c)
	}
}

// serve serves a single accepted connection
func (p *proxy) serve(c net.Conn) {
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
			_ = r.Close()

			p.sm.Lock()
			s.Connected--
			p.sm.Unlock()
		}()

		err = p.copy(c, r, s)
		if err != nil && err != io.EOF {
			log.Printf("error proxying for %s from %s to %s: %s", addr, c.RemoteAddr(), u.addr(), err)
			return
		}

		return
	}
}

// copy copies data between two connections and writes stats
func (p *proxy) copy(client, upstream io.ReadWriter, s *upstreamStats) error {
	ch := make(chan error, 1)

	go p.chanCopy(ch, client, upstream, &s.Out)
	go p.chanCopy(ch, upstream, client, &s.In)

	for i := 0; i < 2; i++ {
		return <-ch
	}

	return nil
}

// chanCopy copies data from source to destination and increments counter
// with the number of bytes copied, it returns an error in the supplied channel
func (p *proxy) chanCopy(ch chan error, dst io.Writer, src io.Reader, counter *uint64) {
	for {
		n, err := io.CopyN(dst, src, copySize)

		p.sm.Lock()
		*counter += uint64(n)
		p.sm.Unlock()

		if err != nil {
			ch <- err
			break
		}
	}
}
