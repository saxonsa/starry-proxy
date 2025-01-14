package main

import (
	"StarryProxy/config"
	"StarryProxy/node"
	"StarryProxy/parameters"
	"StarryProxy/peer"
	"context"
	"log"
)

func main() {
	ctx := context.Background()

	cfg, err := config.InitConfig()
	if err != nil {
		log.Fatalln(err)
	}

	params, err := parameters.InitParameter()
	if err != nil {
		log.Fatalln(params)
	}

	// If we have a destination peer we will start a local server
	if cfg.SuperNode.Id != "" {
		p, err := peer.New(ctx, cfg, peer.NormalNode)
		if err != nil {
			log.Fatalln("Fail to create a peer for normal peer")
		}

		// Make sure our host knows how to reach destPeer
		p.RemotePeer = peer.AddAddrToPeerstore(p.Host, cfg.SuperNode.Id)

		n, err := node.New(*p)
		if err != nil {
			log.Fatalln(err)
		}
		n.ConnectToNet(ctx, cfg, p.RemotePeer)

		n.Serve(ctx, params)
	} else {
		p, err := peer.New(ctx, cfg, peer.MasterNode)
		if err != nil {
			log.Fatalln("Fail to create a peer for the first node")
		}

		// In this case we only need to make sure our host
		// knows how to handle incoming proxied requests from
		// another peer.
		n, err := node.New(*p)

		n.ConnectToNet(ctx, cfg, "")

		// start service
		n.Serve(ctx, params)
	}
}
