package cpuminer

import (
    "fmt"
    "math/rand"
    "sync"

    "github.com/btcsuite/btcd/btcutil"
)

// MiningAddrSource defines an interface that provides mining payout addresses.
// Implementations must be concurrency-safe.
type MiningAddrSource interface {
    // NextAddr returns the next payout address to use.
    NextAddr() btcutil.Address

    // NumAddrs returns the current number of available addresses.
    NumAddrs() int

    // ListEncodedAddrs returns string encodings of all active addresses.
    ListEncodedAddrs() []string

    // AddAddr adds a new address; returns error if duplicate.
    AddAddr(addr btcutil.Address) error

    // RemoveAddr removes an address; returns error if not found.
    RemoveAddr(addr btcutil.Address) error
}

// DefaultAddrSource is a concurrency-safe in-memory mining address store.
type DefaultAddrSource struct {
    mu     sync.RWMutex
    addrs  []btcutil.Address
}

// NewDefaultAddrSource initializes a DefaultAddrSource with optional initial addresses.
func NewDefaultAddrSource(initial []btcutil.Address) *DefaultAddrSource {
    s := &DefaultAddrSource{addrs: make([]btcutil.Address, 0, len(initial))}
    for _, a := range initial {
        // Ignore duplicates from initial input.
        _ = s.AddAddr(a)
    }
    return s
}

func (s *DefaultAddrSource) NextAddr() btcutil.Address {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.addrs[rand.Intn(len(s.addrs))]
}

func (s *DefaultAddrSource) NumAddrs() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.addrs)
}

func (s *DefaultAddrSource) ListEncodedAddrs() []string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    out := make([]string, len(s.addrs))
    for i, a := range s.addrs {
        out[i] = a.EncodeAddress()
    }
    return out
}

func (s *DefaultAddrSource) AddAddr(addr btcutil.Address) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    for _, a := range s.addrs {
        if a.EncodeAddress() == addr.EncodeAddress() {
            return fmt.Errorf("duplicate address detected")
        }
    }
    s.addrs = append(s.addrs, addr)
    return nil
}

func (s *DefaultAddrSource) RemoveAddr(addr btcutil.Address) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    for i, a := range s.addrs {
        if a.EncodeAddress() == addr.EncodeAddress() {
            copy(s.addrs[i:], s.addrs[i+1:])
            s.addrs[len(s.addrs)-1] = nil
            s.addrs = s.addrs[:len(s.addrs)-1]
            return nil
        }
    }
    return fmt.Errorf("mining address not found")
}

