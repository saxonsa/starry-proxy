package peer

import (
	"StarryProxy/config"
	"StarryProxy/ip"
	"StarryProxy/node/service"
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"log"

	ma "github.com/multiformats/go-multiaddr"
)

type Mode int32

const (
	MasterNode = iota
	SuperNode
	NormalNode
)


type Peer struct {
	Mode Mode

	Id peer.ID

	P2PPort int

	Host host.Host

	ProxyAddr ma.Multiaddr

	RemotePeer peer.ID

	Position ip.Position

	Backup bool

	Rate float64

	Service service.Service

	HeartBeatFromMaster bool // 检测是不是在一段时间内收到了来自master的heartbeat

	HeartBeatFromSN bool // 检测是不是在一段时间内收到了来自SN的HeartBeat
}

func New(ctx context.Context, cfg *config.Config, mode Mode) (*Peer, error) {
	// get peer host
	h, err := makeHost(ctx, cfg)
	if err != nil {
		log.Fatalf("fail to make host: %s\n", err)
		return nil, err
	}

	proxyAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", cfg.Proxy.Port))
	if err != nil {
		log.Fatalln(err)
	}

	service := service.InitService()

	return &Peer{
		Mode:     mode,
		Host:     h,
		Id: 	  h.ID(),
		P2PPort:  cfg.P2P.Port,
		Position: cfg.Position,
		RemotePeer: "",
		ProxyAddr: proxyAddr,
		Rate: cfg.Rate,
		Service: *service,
		HeartBeatFromSN: false,
		HeartBeatFromMaster: false,
	}, nil
}

func makeHost(ctx context.Context, cfg *config.Config) (h host.Host, err error) {
	var opt libp2p.Option
	var opts []libp2p.Option

	if opt, err = listenP2PAddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", cfg.P2P.Port)); err != nil {
		return nil, err
	}
	//extMultiAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", cfg.P2P.Port))
	//if err != nil {
	//	log.Printf("Error creating multiaddress: %v\n", err)
	//	return nil, err
	//}
	//addressFactory := func(addrs []ma.Multiaddr) []ma.Multiaddr {
	//	if extMultiAddr != nil {
	//		addrs = append(addrs, extMultiAddr)
	//	}
	//	return addrs
	//}
	//
	//opt = libp2p.AddrsFactory(addressFactory)

	opts = append(opts, opt)

	h, err = libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func listenP2PAddr(addr string) (libp2p.Option, error) {
	return libp2p.ListenAddrStrings(addr), nil
}

// AddAddrToPeerstore parses a peer multiaddress and adds
// it to the given host's peerstore, so it knows how to
// contact it. It returns the peer ID of the remote peer.
func AddAddrToPeerstore(h host.Host, addr string) peer.ID {
	// The following code extracts target's the peer ID from the
	// given multiaddress
	ipfsaddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		log.Printf("fail to parse multiaddr!\n")
		log.Fatalln(err)
	}
	pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		log.Fatalln(err)
	}

	peerid, err := peer.Decode(pid)
	if err != nil {
		log.Fatalln(err)
	}

	// Decapsulate the /ipfs/<peerID> part from the target
	// /ip4/<a.b.c.d>/ipfs/<peer> becomes /ip4/<a.b.c.d>
	targetPeerAddr, _ := ma.NewMultiaddr(
		fmt.Sprintf("/ipfs/%s", peer.Encode(peerid)))
	targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

	// We have a peer ID and a targetAddr so we add
	// it to the peerstore so LibP2P knows how to contact it
	h.Peerstore().AddAddr(peerid, targetAddr, peerstore.PermanentAddrTTL)
	//for index, value := range h.Peerstore().Peers() {
	//	fmt.Printf("peer %d, %s\n", index, value)
	//}
	//log.Printf("num of peers in peerstore: %d\n", h.Peerstore().Peers().Len())
	return peerid
}
