package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func TestOnGetData_FinalBatchDrained_Table(t *testing.T) {
	cfg = &config{DisableBanning: true}

	for _, invSize := range []int{0, 1, 2, 3, 4, 5, 6, 7, 10, 11, 15, 150, 1500, 5000} {
		t.Run(fmt.Sprintf("invSize_%d", invSize), func(t *testing.T) {
			t.Parallel()

			msg := wire.NewMsgGetData()
			doneChans := make([]chan struct{}, invSize)
			for i := range doneChans {
				h := chainhash.Hash{byte(i)}
				msg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &h))
				doneChans[i] = make(chan struct{}, 1)
			}

			s := &server{}
			sp := &serverPeer{
				server: s,
				quit:   make(chan struct{}),
			}

			closed := make([]bool, invSize)
			var mu sync.Mutex

			s.pushInventoryFunc = func(_ *serverPeer, _ *wire.InvVect, doneChan chan<- struct{}) error {
				mu.Lock()
				idx := -1
				for i, wasClosed := range closed {
					if !wasClosed {
						closed[i] = true
						idx = i
						break
					}
				}
				mu.Unlock()

				if idx == -1 {
					t.Fatal("No available index to mark closed")
				}

				go func(i int) {
					time.Sleep(time.Microsecond)
					doneChans[i] <- struct{}{}
					doneChan <- struct{}{}
				}(idx)
				return nil
			}

			done := make(chan struct{})
			go func() {
				sp.OnGetData(nil, msg)
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(time.Millisecond*time.Duration(invSize) + time.Second):
				t.Fatal("OnGetData did not return, possible deadlock")
			}

			for i, ch := range doneChans {
				select {
				case <-ch:
				case <-time.After(100 * time.Millisecond):
					t.Fatalf("doneChan %d not closed", i)
				}
			}
		})
	}
}
