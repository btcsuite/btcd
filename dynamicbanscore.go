package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	// Halflife defines the time (in seconds) by which the transient part
	// of the ban score decays to one half of it's original value.
	Halflife = 60
	lambda   = math.Ln2 / Halflife
	// Lifetime defines the maximum age of the transient part of the ban
	// score to be considered a non-zero score (in seconds).
	Lifetime = 1800
	// BanThreshold defines the maximum allowed ban score before
	// disconnecting and banning misbehaving peers.
	BanThreshold = 100
	// WarnThreshold defines the ban score threshold after which warning
	// messages are emitted whenever the peer misbehaves.
	WarnThreshold = BanThreshold / 2
)

// dynamicBanScore provides dynamic ban scores consisting of a persistent and a
// decaying component. The persistent score can be utilized to create simple
// additive banning policies typically found in other bitcoin node 
// implementations, such as increasing ban score on invalid messages.
//
// The decaying score enables the creation of evasive logic which handles
// misbehaving peers (especially application layer DoS attacks) gracefully
// by disconnecting and banning peers attempting various kinds of flooding.
// dynamicBanScore allows these two approaches to be used in tandem.
//
// Zero value: Values of type dynamicBanScore are immediately ready for use upon
// declaration.
type dynamicBanScore struct {
	lastUnix   int64
	transient  float64
	persistent uint32
	sync.Mutex
}

// String returns the ban score as a human-readable string.
func (s *dynamicBanScore) String() string {
	return fmt.Sprintf("persistent %v + transient %v at %v = %v as of now",
		s.persistent, s.transient, s.lastUnix, s.Int())
}

// Int return the current ban score, the sum of the persistent and decaying
// scores.
//
// This function is safe for concurrent access.
func (s *dynamicBanScore) Int() uint32 {
	return s.int(time.Now())
}

// Increase increases both the persistent and decaying scores by the values
// passed as parameters. The resulting score is returned.
//
// This function is safe for concurrent access.
func (s *dynamicBanScore) Increase(persistent, transient uint32) uint32 {
	s.Lock()
	defer s.Unlock()
	return s.increase(persistent, transient, time.Now())
}

// Reset set both persistent and decaying scores to zero.
//
// This function is safe for concurrent access.
func (s *dynamicBanScore) Reset() {
	s.Lock()
	s.persistent = 0
	s.transient = 0
	s.lastUnix = 0
	s.Unlock()
}

// int return the ban score, the sum of the persistent and decaying scores at a 
// given point in time.
//
// This function is safe for concurrent access. It is intended to be used
// internally and during testing.
func (s *dynamicBanScore) int(t time.Time) uint32 {
	// reduce the amount of time the lock is held
	s.Lock()
	last := s.lastUnix
	tran := s.transient
	pers := s.persistent
	s.Unlock()

	dt := t.Unix() - last
	if tran < 1 || 0 > dt || Lifetime < dt {
		return pers
	}
	return pers + uint32(tran*math.Exp(-1.0*float64(dt)*lambda))
}

// increase increases the persistent, the decaying or both scores by the values
// passed as parameters. The resulting score is calculated as if the action was 
// carreid out at the point time represented by the third paramter. The 
// resulting score is returned.
//
// This function is not safe for concurrent access.
func (s *dynamicBanScore) increase(persistent, transient uint32, t time.Time) uint32 {
	s.persistent += persistent
	tu := t.Unix()
	dt := tu - s.lastUnix

	if transient > 0 {
		if Lifetime < dt {
			s.transient = 0
		} else if s.transient > 1 && 0 < dt {
			s.transient *= math.Exp(-1.0 * float64(dt) * lambda)
		}
		s.transient += float64(transient)
		s.lastUnix = tu
	}
	return s.persistent + uint32(s.transient)
}
