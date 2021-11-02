package peer

import (
	"StarryProxy/config"
	"StarryProxy/ip"
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"log"

	ma "github.com/multiformats/go-multiaddr"
)

type Mode int32

const (
	NeoNode = 0
	SuperNode = 1
	NormalNode = 2
)

type Peer struct {
	Mode Mode

	Id peer.ID

	Host host.Host

	ProxyAddr ma.Multiaddr

	RemotePeer peer.ID

	Position ip.Position

	ComputingPower int
}

func New(ctx context.Context, cfg *config.Config, mode Mode) (*Peer, error) {
	// get peer host
	h, err := makeHost(ctx, cfg)
	if err != nil {
		log.Fatalf("fail to make host: %s\n", err)
		return nil, err
	}

	return &Peer{
		Mode:     mode,
		Host:     h,
		Id: 	  h.ID(),
		Position: cfg.Position,
	}, nil
}

func makeHost(ctx context.Context, cfg *config.Config) (h host.Host, err error) {
	var opt libp2p.Option
	var opts []libp2p.Option

	if opt, err = listenP2PAddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", cfg.P2P.Port)); err != nil {
		return nil, err
	}

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