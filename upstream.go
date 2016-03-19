package zoidbergtcp

import "fmt"

// Upstream is a single upstream server
type Upstream struct {
	host   string
	port   int
	weight int
}

// Addr returns network address of an upstream
func (u Upstream) Addr() string {
	return fmt.Sprintf("%s:%d", u.host, u.port)
}

// String returns string representation of an upstream
func (u Upstream) String() string {
	return u.Addr()
}

// Upstreams is a list of upstreams
type Upstreams []Upstream

func (u Upstreams) Len() int           { return len(u) }
func (u Upstreams) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }
func (u Upstreams) Less(i, j int) bool { return u[i].String() < u[j].String() }
