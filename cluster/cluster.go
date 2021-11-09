package cluster

import (
	"StarryProxy/config"
	"StarryProxy/ip"
	"StarryProxy/peer"
	"log"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

const (
	PeerList = 0
	SNList = 1
)

type Cluster interface {
	GetClusterID() string
	GetClusterSize() int
	GetClusterPosition() ip.Position
	AddPeer(p peer.Peer) error
	FindSuperNodeInPosition(position ip.Position) *peer.Peer
}

type cluster struct {
	Id string `json:"Id"`
	Snid libp2ppeer.ID `json:"Snid"`
	Nodes map[libp2ppeer.ID]peer.Peer `json:"Nodes"`
	Position ip.Position `json:"Position"`
}

func New(p peer.Peer, cfg *config.Config, mode int) (Cluster, error) {
	position := ip.Position{Province: cfg.Position.Province, City: cfg.Position.City}
	cluster := cluster{Snid: p.Id, Nodes: make(map[libp2ppeer.ID]peer.Peer), Position: position}
	cluster.Nodes[p.Id] = p

	// generate cluster id
	cluster.Id = clusterName(shortID(p.Id), mode)
	if mode == SNList {
		log.Println(cluster.Id)
	}
	return &cluster, nil
}

func (c *cluster) GetClusterSize() int {
	return len(c.Nodes)
}

func (c *cluster) GetClusterPosition() ip.Position {
	return c.Position
}

func (c *cluster) AddPeer(p peer.Peer) error {
	c.Nodes[p.Id] = p
	return nil
}

func (c *cluster) FindSuperNodeInPosition(position ip.Position) *peer.Peer {
	for _, p := range c.Nodes {
		if p.Position == position {
			return &p
		}
	}
	return nil
}

func (c *cluster) GetClusterID() string {
	return c.Id
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
