package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
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

func (h *Host) Start() error {
	var err error
	maddr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/" + strconv.Itoa(h.port))
	if err != nil {
		return err
	}

	h.host, err = libp2p.New(h.ctx,
		libp2p.ListenAddrs(maddr),
	)

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
	}
	return err
}

func (h *Host) getNS() string {
	return "ns" + strconv.Itoa(h.port%2)
}
