package build

import (
	"strings"
	"sync/atomic"
)

// AddressPool holds a list of BuildKit daemon addresses and dispenses them in
// round-robin order. Multiple orchestrator goroutines can call Next concurrently
// without coordination — the underlying counter is atomic.
//
// Single-address clusters pay no overhead; multi-address clusters spread builds
// across all available daemons without any locking.
type AddressPool struct {
	addrs  []string
	cursor atomic.Uint64
}

// NewAddressPool creates an AddressPool from a primary address and an optional
// comma-separated extras string. When extras is non-empty its entries take
// precedence over primary; primary is used as the sole address otherwise.
//
// Blank entries in extras are silently dropped.
func NewAddressPool(primary, extras string) *AddressPool {
	p := &AddressPool{}
	if extras != "" {
		for a := range strings.SplitSeq(extras, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				p.addrs = append(p.addrs, a)
			}
		}
	}
	if len(p.addrs) == 0 {
		p.addrs = []string{primary}
	}
	return p
}

// Next returns the next address in round-robin order. It is safe for concurrent use.
func (p *AddressPool) Next() string {
	n := p.cursor.Add(1) - 1
	return p.addrs[n%uint64(len(p.addrs))]
}

// Len returns the number of addresses in the pool.
func (p *AddressPool) Len() int {
	return len(p.addrs)
}
