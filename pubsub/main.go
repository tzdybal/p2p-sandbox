package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

type Host struct {
	port int

	ctx    context.Context
	host   host.Host
	ps     *pubsub.PubSub
	topic  *pubsub.Topic
	subscr *pubsub.Subscription
	//dht  *dht.IpfsDHT
	//disc *discovery.RoutingDiscovery
	addr multiaddr.Multiaddr
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

func (h *Host) Seed(seed *Host) error {
	pid, err := peer.AddrInfoFromP2pAddr(seed.addr)
	if err != nil {
		return err
	}
	return h.host.Connect(h.ctx, *pid)
}

func (h *Host) StartPubSub() error {
	ps, err := pubsub.NewGossipSub(h.ctx, h.host)
	if err != nil {
		return err
	}
	h.ps = ps

	topic, err := h.ps.Join("topic" + strconv.Itoa(h.port%2))
	if err != nil {
		return err
	}
	h.topic = topic

	subscr, err := topic.Subscribe()
	if err != nil {
		return err
	}
	h.subscr = subscr

	log.Println(h.port, "topic:", topic.String(), ", topic peers: ", topic.ListPeers())

	go h.Read()

	return nil
}

func (h *Host) Publish() error {
	err := h.topic.Publish(h.ctx, []byte("ping from"+strconv.Itoa(h.port)))
	log.Println(h.port, "topic peers: ", h.topic.ListPeers())
	return err
}

func (h *Host) Read() error {
	for {
		msg, err := h.subscr.Next(h.ctx)
		if err != nil {
			log.Println(err)
			continue
		}
		if msg.ReceivedFrom != h.host.ID() {
			log.Println(h.port, "recv: ", string(msg.Data))
		}
	}
}

func NewHost(ctx context.Context, port int) *Host {
	return &Host{
		port: port,
		ctx:  ctx,
	}
}

func main() {
	n := 10
	port := 5000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := make([]*Host, n)

	seed := NewHost(ctx, port)
	seed.Start()

	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func(i int) {
			defer wg.Done()
			host := NewHost(ctx, port+i)
			if err := host.Start(); err != nil {
				log.Println("failed to start host:", err)
			}
			if err := host.Seed(seed); err != nil {
				log.Println("failed to use DHT:", err)
			}

			hosts[i-1] = host
		}(i)
	}
	wg.Wait()
	hosts = append(hosts, seed)
	printStats("after connecting to seed", hosts)

	if err := seed.StartPubSub(); err != nil {
		log.Println(err)
	}
	for i := 0; i < len(hosts)-1; i++ {
		if err := hosts[i].StartPubSub(); err != nil {
			log.Println(err)
		}
	}
	time.Sleep(10 * time.Second)
	printStats("after connecting starting pubsub", hosts)

	wg.Add(len(hosts))
	for i := 0; i < len(hosts); i++ {
		go func(i int) {
			defer wg.Done()
			for k := 0; k < 100; k++ {
				if err := hosts[i].Publish(); err != nil {
					log.Println(err)
				}
				time.Sleep(1 * time.Second)
			}
		}(i)
	}
	wg.Wait()
	printStats("after connecting publishing", hosts)

	time.Sleep(10 * time.Second)
}

func printStats(msg string, hosts []*Host) {
	log.Println(msg)
	for _, h := range hosts {
		log.Printf("Host %03d: peers: %d\tconns: %d\n", h.port, len(h.host.Peerstore().Peers()), len(h.host.Network().Peers()))
	}
}
