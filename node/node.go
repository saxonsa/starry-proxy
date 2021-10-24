package node

import (
	"context"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
)

type Node interface {
	New(ctx context.Context, port int)

	Serve(mode Mode)

	ConnectToNet(superNode node)
}

type Mode int32

const (
	NORMAL_NODE	Mode = 0
	SUPER_NODE	Mode = 1
)

type node struct {
	mode Mode

	id peer.ID

	host host.Host

	remotePeer peer.ID

	computingPower int
}
//
//func makeHost(ctx context.Context, cfg config.Config) (host.Host, error) {
//
//}
//
//func New(ctx context.Context, cfg config.Config) (Node, error) {
//	h, err := makeHost(ctx, cfg)
//	if err != nil {
//		log.Fatalf("fail to make host: %s\n", err)
//		return nil, err
//	}
//	return &node{
//
//	}
//}

