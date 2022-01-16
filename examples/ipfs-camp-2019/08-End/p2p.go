package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"

	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	quic "github.com/libp2p/go-libp2p-quic-transport"

	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	discovery "github.com/libp2p/go-libp2p-discovery"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/mr-tron/base58/base58"
	"github.com/multiformats/go-multihash"
	"github.com/sirupsen/logrus"
)

type P2P struct {
	Ctx       context.Context
	Host      host.Host
	KadDHT    *dht.IpfsDHT
	Discovery *discovery.RoutingDiscovery
	PubSub    *pubsub.PubSub
}

func NewP2P() *P2P {
	ctx := context.Background()
	nodehost, kaddht := setupHost(ctx)
	logrus.Debugln("Created the P2P Host and the Kademlia DHT.")

	bootstrapDHT(ctx, nodehost, kaddht)
	logrus.Debugln("Bootstrapped the Kademlia DHT and Connected to Bootstrap Peers")

	routingdiscovery := discovery.NewRoutingDiscovery(kaddht)
	logrus.Debugln("Created the Peer Discovery Service.")

	pubsubhandler := setupPubSub(ctx, nodehost, routingdiscovery)
	logrus.Debugln("Created the PubSub Handler.")

	// Return the P2P object
	return &P2P{
		Ctx:       ctx,
		Host:      nodehost,
		KadDHT:    kaddht,
		Discovery: routingdiscovery,
		PubSub:    pubsubhandler,
	}
}

func setupHost(ctx context.Context) (host.Host, *dht.IpfsDHT) {
	// Set up the host identity options
	prvkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	identity := libp2p.Identity(prvkey)
	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Identity Configuration!")
	}

	// Trace log
	logrus.Traceln("Generated P2P Identity Configuration.")

	// Set up TLS secured TCP transport and options
	tlstransport, err := tls.New(prvkey)
	security := libp2p.Security(tls.ID, tlstransport)
	// transport := libp2p.Transport(tcp.NewTCPTransport)

	transports := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(ws.New),
		libp2p.Transport(quic.NewTransport),
	)
	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Security and Transport Configurations!")
	}

	// Trace log
	logrus.Traceln("Generated P2P Security and Transport Configurations.")

	// Set up host listener address options
	// muladdr, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
	// muladdr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic")

	// muladdr, err := multiaddr.NewMultiaddr("/ip6/::/udp/0/quic")

	// listen := libp2p.ListenAddrs(muladdr)

	listenAddrs := libp2p.ListenAddrStrings(
		"/ip4/0.0.0.0/tcp/0",
		"/ip4/0.0.0.0/tcp/0/ws",
		"/ip4/127.0.0.1/udp/0/quic",
		"/ip6/::/udp/0/quic",
	)

	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Address Listener Configuration!")
	}

	// Trace log
	logrus.Traceln("Generated P2P Address Listener Configuration.")

	// Set up the stream multiplexer and connection manager options
	muxer := libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport)
	conn := libp2p.ConnectionManager(connmgr.NewConnManager(100, 400, time.Minute))

	// Trace log
	logrus.Traceln("Generated P2P Stream Multiplexer, Connection Manager Configurations.")

	// Setup NAT traversal and relay options
	nat := libp2p.NATPortMap()
	relay := libp2p.EnableAutoRelay()

	// Trace log
	logrus.Traceln("Generated P2P NAT Traversal and Relay Configurations.")

	// Declare a KadDHT
	var kaddht *dht.IpfsDHT
	// Setup a routing configuration with the KadDHT
	routing := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		kaddht = setupKadDHT(ctx, h)
		return kaddht, err
	})

	// Trace log
	logrus.Traceln("Generated P2P Routing Configurations.")

	// opts := libp2p.ChainOptions(identity, listen, security, transports, muxer, conn, nat, routing, relay)
	opts := libp2p.ChainOptions(identity, listenAddrs, security, transports, muxer, conn, nat, routing, relay)

	// Construct a new libP2P host with the created options
	// libhost, err := libp2p.New(ctx, opts)
	libhost, err := libp2p.New(opts)

	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Create the P2P Host!")
	}

	// Return the created host and the kademlia DHT
	return libhost, kaddht
}

// A function that generates a Kademlia DHT object and returns it
func setupKadDHT(ctx context.Context, nodehost host.Host) *dht.IpfsDHT {
	// Create DHT server mode option
	dhtmode := dht.Mode(dht.ModeServer)
	// Rertieve the list of boostrap peer addresses
	bootstrappeers := dht.GetDefaultBootstrapPeerAddrInfos()
	// Create the DHT bootstrap peers option
	dhtpeers := dht.BootstrapPeers(bootstrappeers...)

	// Trace log
	logrus.Traceln("Generated DHT Configuration.")

	// Start a Kademlia DHT on the host in server mode
	kaddht, err := dht.New(ctx, nodehost, dhtmode, dhtpeers)
	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Create the Kademlia DHT!")
	}

	// Return the KadDHT
	return kaddht
}

// A function that generates a PubSub Handler object and returns it
// Requires a node host and a routing discovery service.
func setupPubSub(ctx context.Context, nodehost host.Host, routingdiscovery *discovery.RoutingDiscovery) *pubsub.PubSub {
	// Create a new PubSub service which uses a GossipSub router
	pubsubhandler, err := pubsub.NewGossipSub(ctx, nodehost, pubsub.WithDiscovery(routingdiscovery))
	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
			"type":  "GossipSub",
		}).Fatalln("PubSub Handler Creation Failed!")
	}

	// Return the PubSub handler
	return pubsubhandler
}

// A function that bootstraps a given Kademlia DHT to satisfy the IPFS router
// interface and connects to all the bootstrap peers provided by libp2p
func bootstrapDHT(ctx context.Context, nodehost host.Host, kaddht *dht.IpfsDHT) {
	// Bootstrap the DHT to satisfy the IPFS Router interface
	if err := kaddht.Bootstrap(ctx); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Bootstrap the Kademlia!")
	}

	// Trace log
	logrus.Traceln("Set the Kademlia DHT into Bootstrap Mode.")

	// Declare a WaitGroup
	var wg sync.WaitGroup
	// Declare counters for the number of bootstrap peers
	var connectedbootpeers int
	var totalbootpeers int

	// Iterate over the default bootstrap peers provided by libp2p
	for _, peeraddr := range dht.DefaultBootstrapPeers {
		// Retrieve the peer address information
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peeraddr)

		// Incremenent waitgroup counter
		wg.Add(1)
		// Start a goroutine to connect to each bootstrap peer
		go func() {
			// Defer the waitgroup decrement
			defer wg.Done()
			// Attempt to connect to the bootstrap peer
			if err := nodehost.Connect(ctx, *peerinfo); err != nil {
				// Increment the total bootstrap peer count
				totalbootpeers++
			} else {
				// Increment the connected bootstrap peer count
				connectedbootpeers++
				// Increment the total bootstrap peer count
				totalbootpeers++
			}
		}()
	}

	// Wait for the waitgroup to complete
	wg.Wait()

	// Log the number of bootstrap peers connected
	logrus.Debugf("Connected to %d out of %d Bootstrap Peers.", connectedbootpeers, totalbootpeers)
}

// A function that connects the given host to all peers recieved from a
// channel of peer address information. Meant to be started as a go routine.
func handlePeerDiscovery(nodehost host.Host, peerchan <-chan peer.AddrInfo) {
	// Iterate over the peer channel
	for peer := range peerchan {
		// Ignore if the discovered peer is the host itself
		if peer.ID == nodehost.ID() {
			continue
		}

		// Connect to the peer
		nodehost.Connect(context.Background(), peer)
	}
}

// A function that generates a CID object for a given string and returns it.
// Uses SHA256 to hash the string and generate a multihash from it.
// The mulithash is then base58 encoded and then used to create the CID
func generateCID(namestring string) cid.Cid {
	// Hash the service content ID with SHA256
	hash := sha256.Sum256([]byte(namestring))
	// Append the hash with the hashing codec ID for SHA2-256 (0x12),
	// the digest size (0x20) and the hash of the service content ID
	finalhash := append([]byte{0x12, 0x20}, hash[:]...)
	// Encode the fullhash to Base58
	b58string := base58.Encode(finalhash)

	// Generate a Multihash from the base58 string
	mulhash, err := multihash.FromB58String(string(b58string))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate Service CID!")
	}

	// Generate a CID from the Multihash
	cidvalue := cid.NewCidV1(12, mulhash)
	// Return the CID
	return cidvalue
}
