package cluster

import (
	"StarryProxy/config"
	"StarryProxy/ip"
	"StarryProxy/peer"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

const (
	PeerList = 0
	SNList = 1
)

type Cluster interface {
	GetClusterSize() int
	GetClusterPosition() ip.Position
	AddPeer(p peer.Peer) error
	FindSuperNodeInPosition(position ip.Position) *peer.Peer
}

type cluster struct {
	id string
	snid libp2ppeer.ID
	nodes map[libp2ppeer.ID]peer.Peer
	Position ip.Position
}

func New(p peer.Peer, cfg *config.Config, mode int) (Cluster, error) {
	position := ip.Position{Province: cfg.Position.Province, City: cfg.Position.City}
	cluster := cluster{snid: p.Id, nodes: make(map[libp2ppeer.ID]peer.Peer), Position: position}
	cluster.nodes[p.Id] = p

	// generate cluster id
	cluster.id = clusterName(shortID(p.Id), mode)
	return &cluster, nil
}

func (c *cluster) GetClusterSize() int {
	return len(c.nodes)
}

func (c *cluster) GetClusterPosition() ip.Position {
	return c.Position
}

func (c *cluster) AddPeer(p peer.Peer) error {
	c.nodes[p.Id] = p
	return nil
}

func (c *cluster) FindSuperNodeInPosition(position ip.Position) *peer.Peer {
	for _, p := range c.nodes {
		if p.Position == position {
			return &p
		}
	}
	return nil
}

func shortID(p libp2ppeer.ID) string {
	pretty := p.Pretty()
	return pretty[len(pretty)-8:]
}

func clusterName(clusterId string, mode int) string {
	if mode == PeerList {
		return "PeerCluster:" + clusterId
	} else {
		return "SNCluster:" + clusterId
	}
}
