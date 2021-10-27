package cluster

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"log"
	"strconv"
)

type Cluster interface {

}

type Position struct {
	province string
	city string
}

type cluster struct {
	id string
	snid peer.ID
	nodes map[peer.ID]struct{}
	position Position
}

func New(peerId peer.ID, position Position) (Cluster, error) {
	cluster := cluster{snid: peerId, nodes: make(map[peer.ID]struct{}), position: position}

	// generate cluster id
	peerIdInt, err := strconv.Atoi(string(peerId))
	if err != nil {
		log.Fatalf("fail to change string(peerid) to int(peerid): %s\n", err)
		return nil, err
	}
	cluster.id = strconv.Itoa(peerIdInt>>1)

	return &cluster, nil
}


