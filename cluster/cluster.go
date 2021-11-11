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

type ClusterInterface interface {
	GetClusterID() string
	GetClusterSize() int
	GetClusterPosition() ip.Position
	AddPeer(p peer.Peer) error
	FindSuperNodeInPosition(position ip.Position) *peer.Peer
}

type Cluster struct {
	Id string `json:"Id"`
	Snid libp2ppeer.ID `json:"Snid"`
	Nodes map[libp2ppeer.ID]peer.Peer `json:"Nodes"`
	Position ip.Position `json:"Position"`
}

func New(p peer.Peer, cfg *config.Config, mode int, snid libp2ppeer.ID) (Cluster, error) {
	if snid == "" {
		snid = p.Id
	}
	position := ip.Position{Province: cfg.Position.Province, City: cfg.Position.City}
	cluster := Cluster{Snid: snid, Nodes: make(map[libp2ppeer.ID]peer.Peer), Position: position}
	cluster.Nodes[p.Id] = p

	// generate cluster id
	cluster.Id = clusterName(shortID(p.Id), mode)
	if mode == SNList {
		log.Println(cluster.Id)
	}
	return cluster, nil
}

func (c *Cluster) GetClusterSize() int {
	return len(c.Nodes)
}

func (c *Cluster) GetClusterPosition() ip.Position {
	return c.Position
}

func (c *Cluster) AddPeer(p peer.Peer) error {
	c.Nodes[p.Id] = p
	return nil
}

func (c *Cluster) FindSuperNodeInPosition(position ip.Position) *peer.Peer {
	for _, p := range c.Nodes {
		if p.Position == position {
			return &p
		}
	}
	return nil
}

func (c *Cluster) GetClusterID() string {
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
