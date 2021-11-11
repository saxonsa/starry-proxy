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

type AbsCluster interface {
	GetClusterID() string
	GetClusterSize() int
	GetClusterPosition() ip.Position
	AddPeer(p peer.Peer) error
	FindSuperNodeInPosition(position ip.Position) *peer.Peer
}

type Cluster struct {
	Id       string                      `json:"id"`
	Snid     libp2ppeer.ID               `json:"snid"`
	Nodes    []peer.Peer 				 `json:"nodes"`
	Position ip.Position                 `json:"position"`
}

func New(p peer.Peer, cfg *config.Config, mode int) (Cluster, error) {
	position := ip.Position{Province: cfg.Position.Province, City: cfg.Position.City}
	cluster := Cluster{Snid: p.Id, Nodes: make([]peer.Peer, 0), Position: position}
	cluster.Nodes = append(cluster.Nodes, p)
	//cluster.Nodes[p.Id] = p

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
	c.Nodes = append(c.Nodes, p)
	//c.Nodes[p.Id] = p
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
