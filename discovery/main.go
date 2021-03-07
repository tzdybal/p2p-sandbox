package main

import (
	"context"
	"log"
	"time"
)

func main() {
	n := 100
	port := 5000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var hosts []*Host

	seed := NewHost(ctx, port)
	seed.Start()
	seed.UseDHT(nil)

	for i := 1; i <= n; i++ {
		host := NewHost(ctx, port+i)
		if err := host.Start(); err != nil {
			log.Println("failed to start host:", err)
		}
		if err := host.UseDHT(seed); err != nil {
			log.Println("failed to use DHT:", err)
		}

		hosts = append(hosts, host)
	}

	time.Sleep(10 * time.Second)
	printStats("after DHT bootstrapping", hosts)

	for _, h := range hosts {
		if err := h.DiscoveryAdvertise(); err != nil {
			log.Println("failed to advertise:", err)
		}
	}
	time.Sleep(10 * time.Second)
	printStats("after discovery advertisement", hosts)

	for _, h := range hosts {
		if err := h.FindPeers(); err != nil {
			log.Println("failed to find peers:", err)
		}
	}
	time.Sleep(10 * time.Second)
	printStats("after finding peers", hosts)
}

func printStats(msg string, hosts []*Host) {
	log.Println(msg)
	for i, h := range hosts {
		log.Printf("Host %03d (ns: %s): peers: %d\tconns: %d\n", i, h.getNS(), len(h.host.Peerstore().Peers()), len(h.host.Network().Conns()))
	}
}
