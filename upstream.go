package zoidbergtcp

import "fmt"

// upstream is a single upstream server
type upstream struct {
	host   string
	port   int
	weight int
}

// addr returns network address of an upstream
func (u upstream) addr() string {
	return fmt.Sprintf("%s:%d", u.host, u.port)
}

// String returns string representation of an upstream
func (u upstream) String() string {
	return u.addr()
}

// upstreams is a list of upstreams
type upstreams []upstream

func (u upstreams) Len() int           { return len(u) }
func (u upstreams) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }
func (u upstreams) Less(i, j int) bool { return u[i].String() < u[j].String() }
