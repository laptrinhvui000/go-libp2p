package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	// "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	mplex "github.com/libp2p/go-libp2p-mplex"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	// "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	// "github.com/multiformats/go-multiaddr"

	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/ipfs/go-log/v2"

)

var logger = log.Logger("rendezvous")

// type mdnsNotifee struct {
// 	h   host.Host
// 	ctx context.Context
// }

// func (m *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
// 	m.h.Connect(m.ctx, pi)
// }

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transports := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(ws.New),
	)

	muxers := libp2p.ChainOptions(
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
	)

	security := libp2p.Security(tls.ID, tls.New)

	listenAddrs := libp2p.ListenAddrStrings(
		"/ip4/0.0.0.0/tcp/0",
		"/ip4/0.0.0.0/tcp/0/ws",
	)

	// Declare a KadDHT
	// var dht *kaddht.IpfsDHT
	// // Setup a routing configuration with the KadDHT
	// routing := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
	// 	dht = setupKadDHT(ctx, h)
	// 	return dht, err
	// })
	
	var dht *kaddht.IpfsDHT
	newDHT := func(h host.Host) (routing.PeerRouting, error) {
		var err error
		// dht, err = kaddht.New(ctx, h)
		dht = setupKadDHT(ctx, h)

		return dht, err
	}
	routing := libp2p.Routing(newDHT)

	conn := libp2p.ConnectionManager(connmgr.NewConnManager(100, 400, time.Minute))
	nat := libp2p.NATPortMap()
	relay := libp2p.EnableAutoRelay()

	host, err := libp2p.New(
		transports,
		listenAddrs,
		muxers,
		security,
		conn,
		routing,
		nat,
		relay,
	)
	if err != nil {
		panic(err)
	}

	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		panic(err)
	}
	topic, err := ps.Join(pubsubTopic)
	if err != nil {
		panic(err)
	}
	defer topic.Close()
	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	// TODO: Modify this handler to use the protobufs defined in this folder
	go pubsubHandler(ctx, sub)

	for _, addr := range host.Addrs() {
		fmt.Println("Listening on", addr)
	}
	fmt.Println(host.ID())

	// targetAddr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/63785/p2p/QmWjz6xb8v9K4KnYEwP5Yk75k5mMBCehzWFLCvvQpYxF3d")
	// if err != nil {
	// 	panic(err)
	// }

	// targetInfo, err := peer.AddrInfoFromP2pAddr(targetAddr)
	// if err != nil {
	// 	panic(err)
	// }

	// err = host.Connect(ctx, *targetInfo)
	// if err != nil {
	// 	panic(err)
	// }

	// fmt.Println("Connected to", targetInfo.ID)

	// mdns := mdns.NewMdnsService(host, "", &mdnsNotifee{h: host, ctx: ctx})
	// if err := mdns.Start(); err != nil {
	// 	panic(err)
	// }

	err = dht.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	for _, peerAddr := range kaddht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerinfo); err != nil {
				logger.Warning(err)
			} else {
				logger.Info("Connection established with bootstrap node:", *peerinfo)
			}
		}()
	}
	wg.Wait()

	donec := make(chan struct{}, 1)
	go chatInputLoop(ctx, host, topic, donec)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		host.Close()
		os.Exit(0)
	case <-donec:
		host.Close()
	}
}

func setupKadDHT(ctx context.Context, nodehost host.Host) *kaddht.IpfsDHT {
	// Create DHT server mode option
	dhtmode := kaddht.Mode(kaddht.ModeServer)
	// Rertieve the list of boostrap peer addresses
	bootstrappeers := kaddht.GetDefaultBootstrapPeerAddrInfos()
	// Create the DHT bootstrap peers option
	dhtpeers := kaddht.BootstrapPeers(bootstrappeers...)

	// Trace log

	// Start a Kademlia DHT on the host in server mode
	kaddht, err := kaddht.New(ctx, nodehost, dhtmode, dhtpeers)
	// Handle any potential error
	if err != nil {
		panic(err)
	}
	// Return the KadDHT
	return kaddht
}