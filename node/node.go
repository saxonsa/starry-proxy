package node

import (
	"StarryProxy/ip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"StarryProxy/cluster"
	"StarryProxy/config"
	"StarryProxy/peer"
	"StarryProxy/protocol"

	"github.com/diandianl/p2p-proxy/relay"
	"github.com/elazarl/goproxy"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	gostream "github.com/libp2p/go-libp2p-gostream"
	manet "github.com/multiformats/go-multiaddr-net"
)

type Node interface {
	Serve(ctx context.Context, cfg *config.Config)

	ConnectToNet(ctx context.Context, cfg *config.Config, superNode libp2ppeer.ID)
}

type node struct {
	self peer.Peer

	peerList cluster.Cluster

	snList	cluster.Cluster
}


type listener struct {
	net.Listener
}

func New(peer peer.Peer) (Node, error) {
	node := node{self: peer}
	return &node, nil
}

// msg sent from peer to super node when entry the p2p net
type peerInfo struct {
	PeerAddr string
	Position ip.Position
	BandWidth int
}

type message struct {
	Operand int `json:"operand"`
	peerInfo peerInfo
	SuperClusterSnid string
	PeerClusterSnid string
	SnList cluster.ClusterInterface
}

func (n *node) ConnectToNet(ctx context.Context, cfg *config.Config, snid libp2ppeer.ID) {
	// The first node entered the p2p net
	if snid == "" {
		// init 2 clusters
		n.snList, _ = cluster.New(n.self, cfg, cluster.SNList, "")
		n.peerList, _ = cluster.New(n.self, cfg, cluster.PeerList, "")
		return
	}

	// build a stream which tags "NewNodeEntryProtocol"
	conn, _ := gostream.Dial(ctx, n.self.Host, snid, protocol.NewNodeProtocol)

	// send self peer Info to supernode
	peerInfo := peerInfo{PeerAddr: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", cfg.P2P.Port, n.self.Id.Pretty()),
		Position: n.self.Position}
	peerInfoJson, _ := json.Marshal(peerInfo)
	conn.Write(peerInfoJson)

	for {
		msg := message{}
		buffer := make([]byte, 1024)
		len, err := conn.Read(buffer)
		err = json.Unmarshal(buffer[:len], &msg)
		if err != nil {
			log.Println("test here")
			log.Printf("fail to convert json to struct format: %s", err)
		}

		fmt.Printf("ope: %d\n", msg.Operand)

		switch msg.Operand {
			case protocol.EXIT: {
				return
			}
			case protocol.AssignSelfAsSupernode: {
				go n.StartNewNodeEntryService(cfg)
				n.self.Mode = peer.SuperNode
				n.peerList, _ = cluster.New(n.self, cfg, cluster.PeerList, "")
				//n.snList, _ = cluster.New(n.self, cfg, cluster.SNList, libp2ppeer.ID(msg.SuperClusterSnid))
			}
			//case protocol.SuperClusterNode: {
			//	fmt.Println("tet..sa")
			//}
		}
	}
}

func (n *node) Serve(ctx context.Context, cfg *config.Config) {
	// 启动proxy service, 监听 gostream <commonProtocol>, 将收到的http请求用goproxy处理掉
	go n.StartProxyService()

	fmt.Println("Proxy server is ready")
	fmt.Println("libp2p-peer addresses:")
	for _, a := range n.self.Host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, libp2ppeer.Encode(n.self.Id))
	}

	if n.self.Mode == peer.SuperNode {
		// start a service waiting for the node to enter the cluster
		go n.StartNewNodeEntryService(cfg)
	}
	// 监听设置好的proxy端口, 将http请求转发到这个端口上, 然后端口将stream转发给remote proxy
	// 如果没有remote peer, 自己处理端口的请求
	n.listenOnProxy(ctx)
}

func (n *node) listenOnProxy(ctx context.Context) {
	_, serveArgs, _ := manet.DialArgs(n.self.ProxyAddr)
	fmt.Println("proxy listening on ", serveArgs)
	l, err := net.Listen("tcp", serveArgs)
	if err != nil {
		log.Fatalln(err)
	}
	listener := &listener{l}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Fatalln(err)
			}
			go func() {
				err := n.connHandler(ctx, conn)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	}()

	<-ctx.Done()
}

func (n *node) connHandler(ctx context.Context, conn net.Conn) error {
	if n.self.RemotePeer == "" {
		return nil
	}
	// 将stream转发给remote peer
	stream, err := n.self.Host.NewStream(ctx, n.self.RemotePeer, protocol.HTTPProxyProtocol)
	if err != nil {
		log.Fatalln(err)
	}
	err = relay.CloseAfterRelay(conn, stream)
	if err != nil {
		return err
	}
	return nil
}

func (n *node) StartProxyService() {
	l, err := gostream.Listen(n.self.Host, protocol.HTTPProxyProtocol)
	if err != nil {
		log.Println(err)
	}
	defer l.Close()
	proxy := goproxy.NewProxyHttpServer()
	s := &http.Server{Handler: proxy}
	err = s.Serve(l)
	if err != nil {
		return
	}
}

func (n *node) StartNewNodeEntryService(cfg *config.Config) {
	l, err := gostream.Listen(n.self.Host, protocol.NewNodeProtocol)
	if err != nil {
		log.Println(err)
	}
	defer l.Close()

	// handle peer connection
	for {
		conn, _ := l.Accept()
		defer conn.Close()

		go func() {
			// receive peer information
			pInfo := peerInfo{}
			buffer := make([]byte, 1024)
			len, err := conn.Read(buffer)
			err = json.Unmarshal(buffer[:len], &pInfo)
			if err != nil {
				log.Println("tesT???")
				log.Printf("fail to convert json to struct format: %s", err)
			}

			// put the normal node in the right cluster
			if pInfo.Position == n.peerList.GetClusterPosition() {
				// at the same position - put in the same cluster - conn directly to sn
				err := n.peerList.AddPeer(peer.Peer{
					Id: libp2ppeer.ID(pInfo.PeerAddr),
					Position: pInfo.Position,
					Mode: peer.NormalNode,
				})
				if err != nil {
					fmt.Printf("fail to add a peer to peerList: %s", err)
				}

				// 如果supernode没有remotepeer, 立即将连过来的这个作为remote peer
				if n.self.RemotePeer == "" {
					n.self.RemotePeer = peer.AddAddrToPeerstore(n.self.Host, pInfo.PeerAddr)
				}
			} else {
				// find if the supernode of the right cluster exists
				p := n.snList.FindSuperNodeInPosition(pInfo.Position)
				if p == nil {
					remotePeer := peer.AddAddrToPeerstore(n.self.Host, pInfo.PeerAddr)

					// 将peer加入到supernode list
					n.snList.AddPeer(peer.Peer{
						Id: remotePeer,
						Position: pInfo.Position,
						Mode: peer.NormalNode,
					})

					// 让peer将自己作为supernode
					msg := message{Operand: protocol.AssignSelfAsSupernode}
					msgJson, err := json.Marshal(msg)
					if err != nil {
						fmt.Println(err)
					}
					conn.Write(msgJson)

					//msg := message{Operand: protocol.AssignSelfAsSupernode, SuperClusterSnid: string(n.snList.Snid)}
					//msgJson, _ := json.Marshal(&msg)
					//conn.Write(msgJson)

					//// 给peer发送cluster的信息
					//for pid, p := range n.snList.Nodes {
					//	msg = message{Operand: protocol.SuperClusterNode, peerInfo: peerInfo{
					//		PeerAddr: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", cfg.P2P.Port, pid.Pretty()),
					//		Position: p.Position,
					//		BandWidth: p.BandWidth,
					//	}}
					//	msgJson, _ = json.Marshal(msg)
					//	conn.Write(msgJson)
					//}
				} else {
					// found

					fmt.Println("found node")
				}
			}
			message := message{Operand: protocol.EXIT}
			msgJson, _ := json.Marshal(&message)
			conn.Write(msgJson)
		}()
	}
}