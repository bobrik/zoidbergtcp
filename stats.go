package zoidbergtcp

// stat represents stats about upstreams
type stats struct {
	Servers map[string]proxyStats `json:"servers"`
}

// proxyStats represents stats about upstreams of a specific proxy
type proxyStats struct {
	Upstreams map[string]upstreamStats `json:"upstreams"`
}

// upstreamStats represents stats about a single upstream
type upstreamStats struct {
	In          uint64 `json:"in"`
	Out         uint64 `json:"out"`
	Errors      uint64 `json:"errors"`
	Connected   uint64 `json:"connected"`
	Connections uint64 `json:"connections"`
}
