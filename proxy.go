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

// newProxy creates a new tcp proxy
func newProxy(listen string) (*proxy, error) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}

	return &proxy{
		listener:  listener,
		mutex:     sync.Mutex{},
		upstreams: []upstream{},
	}, nil
}

// proxy represents a tcp proxy with upstreams
type proxy struct {
	mutex     sync.Mutex
	listener  net.Listener
	upstreams upstreams
}

// setState sets state for the proxy based on servers and their versions
func (p *proxy) setState(port int, servers []application.Server, versions state.Versions) {
	p.mutex.Lock()

	upstreams := upstreams{}

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

		upstreams = append(upstreams, upstream{
			host:   s.Host,
			port:   s.Ports[port],
			weight: w,
		})
	}

	sort.Sort(upstreams)

	if !reflect.DeepEqual(upstreams, p.upstreams) {
		p.upstreams = upstreams

		log.Printf("updated upstreams for %s: %s\n", p.listener.Addr(), upstreams)
	}

	p.mutex.Unlock()
}

// start starts main proxy loop
func (p *proxy) start() {
	addr := p.listener.Addr()

	for {
		c, err := p.listener.Accept()
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

	addr := p.listener.Addr()

	p.mutex.Lock()
	upstreams := make([]upstream, len(p.upstreams))
	copy(upstreams, p.upstreams)
	p.mutex.Unlock()

	// TODO: proper iterator accounting weights and current # of connections
	for i := range upstreams {
		j := rand.Intn(i + 1)
		upstreams[i], upstreams[j] = upstreams[j], upstreams[i]
	}

	for _, u := range upstreams {
		log.Printf("connecting to %s for %s..\n", u, addr)
		r, err := net.Dial("tcp", u.addr())
		if err != nil {
			log.Printf("error connecting for %s to %s: %s", addr, u.addr(), err)
			continue
		}

		defer func() {
			_ = r.Close()
		}()

		err = p.copy(c, r)
		if err != nil {
			log.Printf("error proxying for %s from %s to %s: %s", addr, c.RemoteAddr(), u.addr(), err)
			return
		}

		return
	}
}

// copy copies data between two connections
func (p *proxy) copy(c, u net.Conn) error {
	ch := make(chan error, 1)

	go chanCopy(ch, c, u)
	go chanCopy(ch, u, c)

	for i := 0; i < 2; i++ {
		return <-ch
	}

	return nil
}

// chanCopy copies data between read writers in one direction
// and returns an error in the supplied channel
func chanCopy(ch chan error, dst, src io.ReadWriter) {
	_, err := io.Copy(dst, src)
	ch <- err
}
