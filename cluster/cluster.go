package cluster

import (
	"log"

	"StarryProxy/config"
	"StarryProxy/peer"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type Cluster interface {
	GetClusterSize() int
}

type Position struct {
	province string
	city string
}

type cluster struct {
	id string
	snid libp2ppeer.ID
	nodes map[libp2ppeer.ID]peer.Peer
	position Position
}

func New(p peer.Peer, cfg *config.Config) (Cluster, error) {
	position := Position{province: cfg.Position.Province, city: cfg.Position.City}
	cluster := cluster{snid: p.Id, nodes: make(map[libp2ppeer.ID]peer.Peer), position: position}
	cluster.nodes[p.Id] = p

	// generate cluster id
	cluster.id = clusterName(shortID(p.Id))

	log.Println(cluster.id)

	return &cluster, nil
}

func (c *cluster) GetClusterSize() int {
	return len(c.nodes)
}

func shortID(p libp2ppeer.ID) string {
	pretty := p.Pretty()
	return pretty[len(pretty)-8:]
}

func clusterName(clusterId string) string {
	return "cluster:" + clusterId
}
