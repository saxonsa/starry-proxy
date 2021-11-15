package cluster

import (
	"StarryProxy/config"
	"StarryProxy/ip"
	"StarryProxy/peer"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	"math/rand"
)

const (
	PeerList = iota
	SNList
)

type AbsCluster interface {
	GetClusterID() string
	GetClusterSize() int
	GetClusterPosition() ip.Position
	AddPeer(p peer.Peer) error
	RemovePeer(pid libp2ppeer.ID) string
	FindSuperNodeInPosition(position ip.Position) *peer.Peer
	FindRandomPeer(pid libp2ppeer.ID) *peer.Peer
	FindSecondRankPeer() (*peer.Peer, string)
}

type Cluster struct {
	Id       string                      `json:"id"`
	Snid     libp2ppeer.ID               `json:"snid"`
	Backup	 libp2ppeer.ID				 `json:"backup"`
	Nodes    []peer.Peer 				 `json:"nodes"`
	Position ip.Position                 `json:"position"`
}

func New(p peer.Peer, cfg *config.Config, mode int) (Cluster, error) {
	position := ip.Position{Province: cfg.Position.Province, City: cfg.Position.City}
	cluster := Cluster{Snid: p.Id, Nodes: make([]peer.Peer, 0), Position: position}
	cluster.Nodes = append(cluster.Nodes, p)

	// generate cluster id
	cluster.Id = clusterName(p.Position.Province + " " + p.Position.City, int(p.Mode))

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
	return nil
}

func (c *Cluster) RemovePeer(pid libp2ppeer.ID) string {
	for index, p := range c.Nodes {
		if p.Id == pid {
			c.Nodes = append(c.Nodes[:index], c.Nodes[index+1:]...)
			return ""
		}
	}
	return "Peer Not Found"
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

func (c *Cluster) FindRandomPeer(pid libp2ppeer.ID) *peer.Peer {
	for {
		// 真的就是很随机... -_-
		r := c.GetClusterSize() - 1
		if r == 0 {
			return nil
		}

		index := rand.Intn(r)
		if c.Nodes[index].Id == pid {
			continue
		}
		return &c.Nodes[index]
	}
}

func (c *Cluster) FindSecondRankPeer() (*peer.Peer, string) {
	maxBandwidth := 0
	index := 0
	for i, p := range c.Nodes {
		if p.Id != c.Snid {
			if p.BandWidth > maxBandwidth {
				index = i
				maxBandwidth = p.BandWidth
			}
		}
	}

	if c.Nodes[index].Id != c.Snid {
		return &c.Nodes[index], ""
	}

	return &c.Nodes[index], "Cannot find proper second node"
}

func clusterName(position string, mode int) string {
	if mode == PeerList {
		return "PeerCluster:" + position
	} else {
		return "SNCluster:" + position
	}
}
