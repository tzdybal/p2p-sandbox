package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/control"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
)

type Host struct {
	port int

	ctx  context.Context
	host host.Host
	dht  *dht.IpfsDHT
	disc *discovery.RoutingDiscovery
	addr multiaddr.Multiaddr
}

func NewHost(ctx context.Context, port int) *Host {
	return &Host{
		port: port,
		ctx:  ctx,
	}
}

type Gater struct {
	h host.Host
}

// InterceptPeerDial tests whether we're permitted to Dial the specified peer.
//
// This is called by the network.Network implementation when dialling a peer.
func (g *Gater) InterceptPeerDial(p peer.ID) (allow bool) {
	protos, err := g.h.Peerstore().GetProtocols(p)
	if err != nil {
		return false
	}
	log.Println(p, "PeerDial:", protos)

	return true
}

// InterceptAddrDial tests whether we're permitted to dial the specified
// multiaddr for the given peer.
//
// This is called by the network.Network implementation after it has
// resolved the peer's addrs, and prior to dialling each.
func (g *Gater) InterceptAddrDial(p peer.ID, _ multiaddr.Multiaddr) (allow bool) {
	protos, err := g.h.Peerstore().GetProtocols(p)
	if err != nil {
		return false
	}
	log.Println(p, "AddrDial:", protos)

	return true
}

// InterceptAccept tests whether an incipient inbound connection is allowed.
//
// This is called by the upgrader, or by the transport directly (e.g. QUIC,
// Bluetooth), straight after it has accepted a connection from its socket.
func (g *Gater) InterceptAccept(_ network.ConnMultiaddrs) (allow bool) {
	return true
}

// InterceptSecured tests whether a given connection, now authenticated,
// is allowed.
//
// This is called by the upgrader, after it has performed the security
// handshake, and before it negotiates the muxer, or by the directly by the
// transport, at the exact same checkpoint.
func (g *Gater) InterceptSecured(_ network.Direction, p peer.ID, _ network.ConnMultiaddrs) (allow bool) {
	protos, err := g.h.Peerstore().GetProtocols(p)
	if err != nil {
		return false
	}
	log.Println(p, "Secured:", protos)

	return true
}

// InterceptUpgraded tests whether a fully capable connection is allowed.
//
// At this point, the connection a multiplexer has been selected.
// When rejecting a connection, the gater can return a DisconnectReason.
// Refer to the godoc on the ConnectionGater type for more information.
//
// NOTE: the go-libp2p implementation currently IGNORES the disconnect reason.
func (g *Gater) InterceptUpgraded(c network.Conn) (allow bool, reason control.DisconnectReason) {
	protos, err := g.h.Peerstore().GetProtocols(c.RemotePeer())
	if err != nil {
		return false, 0
	}
	log.Println(c.RemotePeer(), "Upgraded:", protos)

	return true, 0
}

func (h *Host) Start() error {
	var err error
	maddr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/" + strconv.Itoa(h.port))
	if err != nil {
		return err
	}

	gater := &Gater{}
	h.host, err = libp2p.New(h.ctx,
		libp2p.ListenAddrs(maddr),
		libp2p.ConnectionManager(connmgr.NewConnManager(3, 10, 0*time.Second)),
	)
	gater.h = h.host

	for _, a := range h.host.Addrs() {
		full := fmt.Sprintf("%s/p2p/%s", a, h.host.ID())
		log.Println("listening on address:", full)
		h.addr, _ = multiaddr.NewMultiaddr(full)
	}

	return nil
}

func (h *Host) UseDHT(seed *Host) error {
	var err error
	var opts []dht.Option

	opts = append(opts, dht.Mode(dht.ModeServer))
	if seed != nil {
		ai, err := peer.AddrInfoFromP2pAddr(seed.addr)
		if err != nil {
			return fmt.Errorf("invalid seed address: %w", err)
		}
		opts = append(opts, dht.BootstrapPeers(*ai))
	}

	h.dht, err = dht.New(h.ctx, h.host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}
	err = h.dht.Bootstrap(h.ctx)
	if err != nil {
		return fmt.Errorf("failed to bootstrap DHT routing table: %w", err)
	}

	return nil
}

func (h *Host) DiscoveryAdvertise() error {
	h.disc = discovery.NewRoutingDiscovery(h.dht)
	_, err := h.disc.Advertise(h.ctx, h.getNS(), discovery.Limit(10))
	return err
}

func (h *Host) FindPeers() error {
	peerCh, err := h.disc.FindPeers(h.ctx, h.getNS())
	for _ = range peerCh {
		fmt.Printf(".")
	}
	fmt.Printf("\n")
	return err
}

func (h *Host) getNS() string {
	return "ns" + strconv.Itoa(h.port%2)
}
