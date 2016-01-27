package peer_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

func TestConnectPeer(t *testing.T) {
	params := &chaincfg.MainNetParams

	peerIPs := []net.IP{}
	for _, seed := range params.DNSSeeds {

		ips, err := net.LookupIP(seed)
		if err != nil {
			t.Error(err)
		}
		peerIPs = append(peerIPs, ips...)

	}

	verAck := make(chan struct{}, 1)
	addrMsgs := make(chan *wire.MsgAddr, 1)
	peerCfg := &peer.Config{
		UserAgentName:    "rtwire",
		UserAgentVersion: "0.0.1",
		ChainParams:      params,
		Services:         0,
		Listeners: peer.MessageListeners{
			OnVersion: func(p *peer.Peer, msg *wire.MsgVersion) {
			},
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				verAck <- struct{}{}
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				addrMsgs <- msg
			},
			OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
				/*
					for _, invVect := range msg.InvList {
						fmt.Println(invVect.Type)
					}
				*/
			},
			/*
				OnRead: func(p *peer.Peer, bytesRead int,
					msg wire.Message, err error) {
					fmt.Println("read", msg.Command(), err)
				},
			*/
			OnWrite: func(p *peer.Peer, bytesWritten int,
				msg wire.Message, err error) {
			},
		},
	}

	hostPort := net.JoinHostPort(peerIPs[23].String(), params.DefaultPort)
	peerAddr, err := net.ResolveTCPAddr("tcp", hostPort)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := net.DialTCP("tcp", &net.TCPAddr{}, peerAddr)
	if err != nil {
		t.Fatal(err)
	}

	p, err := peer.NewOutboundPeer(peerCfg, conn)
	if err != nil {
		t.Fatal(err)
	}

	if err := <-p.QueueMessage(wire.NewMsgGetAddr()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-addrMsgs:
		fmt.Println("got addrmsg")
	}
	p.Shutdown()
	p.WaitForShutdown()
}
