package main

import (
	"StarryProxy/config"
	"StarryProxy/node"
	"StarryProxy/peer"

	"context"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p-core/host"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
)


// addAddrToPeerstore parses a peer multiaddress and adds
// it to the given host's peerstore, so it knows how to
// contact it. It returns the peer ID of the remote peer.
func addAddrToPeerstore(h host.Host, addr string) libp2ppeer.ID {
	// The following code extracts target's the peer ID from the
	// given multiaddress
	ipfsaddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		log.Fatalln(err)
	}
	pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		log.Fatalln(err)
	}

	peerid, err := libp2ppeer.Decode(pid)
	if err != nil {
		log.Fatalln(err)
	}

	// Decapsulate the /ipfs/<peerID> part from the target
	// /ip4/<a.b.c.d>/ipfs/<peer> becomes /ip4/<a.b.c.d>
	targetPeerAddr, _ := ma.NewMultiaddr(
		fmt.Sprintf("/ipfs/%s", libp2ppeer.Encode(peerid)))
	targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

	// We have a peer ID and a targetAddr so we add
	// it to the peerstore so LibP2P knows how to contact it
	h.Peerstore().AddAddr(peerid, targetAddr, peerstore.PermanentAddrTTL)
	return peerid
}

func main() {
	ctx := context.Background()

	cfg, err := config.InitConfig()
	if err != nil {
		log.Fatalln(err)
	}

	// If we have a destination peer we will start a local server
	if cfg.SuperNode.Id != "" {
		p, err := peer.New(ctx, cfg, peer.NormalNode)
		if err != nil {
			log.Fatalln("Fail to create a peer for normal peer")
		}

		// Make sure our host knows how to reach destPeer
		destPeerID := addAddrToPeerstore(p.Host, cfg.SuperNode.Id)
		proxyAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", cfg.Proxy.Port))
		if err != nil {
			log.Fatalln(err)
		}

		p.ProxyAddr = proxyAddr
		p.RemotePeer = destPeerID

		n, err := node.New(*p)
		if err != nil {
			log.Fatalln(err)
		}
		n.Serve(ctx, cfg)
	} else {
		p, err := peer.New(ctx, cfg, peer.SuperNode)
		if err != nil {
			log.Fatalln("Fail to create a peer for supernode")
		}
		// In this case we only need to make sure our host
		// knows how to handle incoming proxied requests from
		// another peer.
		n, err := node.New(*p)
		n.Serve(ctx, cfg)
	}
}
