package node

import (
	"StarryProxy/ip"
	"StarryProxy/parameters"
	"StarryProxy/request"
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

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
	Serve(ctx context.Context, cfg *config.Config, params *parameters.Parameter)

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
	Id libp2ppeer.ID
	PeerAddr string
	Position ip.Position
	BandWidth int
	P2PPort int
}

type Message struct {
	Operand int
	SnList cluster.Cluster
	PeerList cluster.Cluster
	ExistedSupernode peer.Peer // 如果访问的peer所在的区域已经有supernode, 将supernode的信息发给peer
	Sender peer.Peer
}

func (n *node) ConnectToNet(ctx context.Context, cfg *config.Config, snid libp2ppeer.ID) {
	// The first node entered the p2p net
	if n.self.Mode == peer.SSPNode {
		fmt.Println("enter p2p net test")

		if cfg.Demo {
			params := make(map[string]string)
			params["mode"] = string(rune(peer.SSPNode))
			params["peer_id"] = string(n.self.Id)
			params["peer_name"] = cfg.Name
			params["province"] = n.self.Position.Province
			params["city"] = n.self.Position.City
			request.Post("/enter_p2p_net", params)
		}

		// init 2 clusters
		n.snList, _ = cluster.New(n.self, cfg, cluster.SNList)
		n.peerList, _ = cluster.New(n.self, cfg, cluster.PeerList)
		return
	}

	// build a stream which tags "NewNodeEntryProtocol"
	conn, err := gostream.Dial(ctx, n.self.Host, snid, protocol.NewNodeProtocol)
	if err != nil {
		fmt.Println("dail new protocol failed")
		fmt.Println(err)
	}

	// send self peer Info to supernode
	peerInfo := peerInfo{Id: n.self.Id, PeerAddr: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", cfg.P2P.Port, n.self.Id),
		Position: n.self.Position, P2PPort: cfg.P2P.Port}
	peerInfoJson, err := json.Marshal(peerInfo)
	if err != nil {
		fmt.Println("failed to marshal peerInfo")
	}
	_, err = conn.Write(peerInfoJson)
	if err != nil {
		fmt.Println("write error")
		fmt.Println(err)
		return
	}

	for {
		msg := DecodeGobObjectIntoMessage(conn)

		switch msg.Operand {
			case protocol.EXIT: {
				return
			}
			case protocol.PeerList: {
				// copy msg.PeerList into n.SnList
				n.peerList = CopyCluster(n.peerList, msg.PeerList)

				// 将peerList中和自己没有连接的连起来
				n.ConnectUnconnectedClusterPeer(n.peerList)
			}
			case protocol.AllClusterList: {
				n.snList = CopyCluster(n.snList, msg.SnList)
				n.peerList = CopyCluster(n.peerList, msg.PeerList)
				n.ConnectUnconnectedClusterPeer(n.snList)
				n.ConnectUnconnectedClusterPeer(n.peerList)
			}
			case protocol.SNList: {
				n.snList = CopyCluster(n.snList, msg.SnList)

				// 将snlist中和自己没有连接起来的连起来
				n.ConnectUnconnectedClusterPeer(n.snList)
			}
			case protocol.AssignSelfAsSupernode: {
				go n.StartNewNodeEntryService()
				n.peerList, _ = cluster.New(n.self, cfg, cluster.PeerList)

				// copy msg.Snlist into n.snlist
				n.snList = CopyCluster(n.snList, msg.SnList)

				// 将snlist中和自己没有连接起来的连起来
				n.ConnectUnconnectedClusterPeer(n.snList)
			}
			case protocol.ExistedSupernodeInSelfCluster: {
				// 获取supernode的peer信息
				dest := peer.AddAddrToPeerstore(
					n.self.Host,
					fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", msg.ExistedSupernode.P2PPort, msg.ExistedSupernode.Id),
				)

				// 改变remote peer成现在的supernode
				n.self.RemotePeer = dest

				// 以自己cluster拥有的supernode接入p2p net
				n.ConnectToNet(ctx, cfg, dest)
				return
			}
		}
	}
}

func (n *node) Serve(ctx context.Context, cfg *config.Config, params *parameters.Parameter) {

	// do something when node quit
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		fmt.Println("node quit!")
		os.Exit(0)
	}()

	// 启动proxy service, 监听 gostream <commonProtocol>, 将收到的http请求用goproxy处理掉
	go n.StartProxyService()

	fmt.Println("Proxy server is ready")
	fmt.Println("libp2p-peer addresses:")
	for _, a := range n.self.Host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, libp2ppeer.Encode(n.self.Id))
	}

	go n.ListenUpdatedCluster()
	go n.StartAliveTestAck()
	go n.StartNewNodeEntryService()

	switch n.self.Mode {
		case peer.SSPNode: {
			go n.StartAliveTest(ctx, cluster.SNList, params.Period.SnList)
			go n.StartAliveTest(ctx, cluster.PeerList, params.Period.PeerList)
		}
		case peer.SuperNode: {
			go n.StartAliveTest(ctx, cluster.PeerList, params.Period.PeerList)
		}
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

func (n *node) StartNewNodeEntryService() {
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
				log.Printf("fail to convert json to struct format: %s", err)
			}

			// put the normal node in the right cluster
			if pInfo.Position == n.peerList.GetClusterPosition() {
				if n.self.Mode == peer.NormalNode {
					supernode := n.peerList.FindSuperNodeInPosition(n.peerList.Position)
					msg := Message{Operand: protocol.ExistedSupernodeInSelfCluster, ExistedSupernode: *supernode}
					conn.Write(EncodeMessageToGobObject(msg).Bytes())
					return
				}

				fmt.Println("put the node in my cluster")
				// at the same position - put in the same cluster - conn directly to sn
				err := n.peerList.AddPeer(peer.Peer{
					Id: pInfo.Id,
					Position: pInfo.Position,
					Mode: peer.NormalNode,
					P2PPort: pInfo.P2PPort,
				})
				if err != nil {
					fmt.Printf("fail to add a peer to peerList: %s", err)
				}

				// 如果supernode没有remotepeer, 立即将连过来的这个作为remote peer
				if n.self.RemotePeer == "" {
					n.self.RemotePeer = peer.AddAddrToPeerstore(n.self.Host, pInfo.PeerAddr)
				}

				// 将peerList发给这个peer
				peerInfoList := ConstructSendableNodesList(n.peerList)
				SNInfoList := ConstructSendableNodesList(n.snList)
				msg := Message{
					Operand: protocol.AllClusterList,
					PeerList: cluster.Cluster{
						Id: n.peerList.Id,
						Snid: n.peerList.Snid,
						Nodes: peerInfoList,
						Position: n.peerList.Position,
					},
					SnList: cluster.Cluster{
						Id: n.peerList.Id,
						Snid: n.peerList.Snid,
						Nodes: SNInfoList,
						Position: n.peerList.Position,
					},
				}
				conn.Write(EncodeMessageToGobObject(msg).Bytes())

			} else {
				// find if the supernode of the right cluster exists
				p := n.snList.FindSuperNodeInPosition(pInfo.Position)

				if p == nil { // not exists 让peer将自己作为supernode
					remotePeer := peer.AddAddrToPeerstore(n.self.Host, pInfo.PeerAddr)

					// 将peer加入到supernode list
					n.snList.AddPeer(peer.Peer{
						Id: remotePeer,
						Position: pInfo.Position,
						Mode: peer.NormalNode,
						P2PPort: pInfo.P2PPort,
					})

					// 将snlist的信息传给这个peer
					peerInfoList := ConstructSendableNodesList(n.snList)

					msg := Message{
						Operand: protocol.AssignSelfAsSupernode,
						SnList: cluster.Cluster{
							Id: n.snList.Id,
							Snid: n.snList.Snid,
							Nodes: peerInfoList, // nodes只有部分属性可以传过去
							Position: n.snList.Position,
						},
					}
					conn.Write(EncodeMessageToGobObject(msg).Bytes())
				} else {
					// found a supernode in current peer's position(cluster)
					msg := Message{Operand: protocol.ExistedSupernodeInSelfCluster, ExistedSupernode: *p}
					conn.Write(EncodeMessageToGobObject(msg).Bytes())

					return
				}
			}
			msg := Message{Operand: protocol.EXIT}
			conn.Write(EncodeMessageToGobObject(msg).Bytes())
		}()
	}
}

// ListenUpdatedCluster 更新最新传来的cluster
func (n *node) ListenUpdatedCluster() {
	l, err := gostream.Listen(n.self.Host, protocol.UpdateClusterProtocol)
	if err != nil {
		log.Println(err)
	}
	defer l.Close()
	for {
		conn, _ := l.Accept()
		go func() {
			msg := DecodeGobObjectIntoMessage(conn)
			switch msg.Operand {
			case protocol.SNList: {
				n.snList = CopyCluster(n.snList, msg.SnList)

				// 将snlist中和自己没有连接起来的连起来
				n.ConnectUnconnectedClusterPeer(n.snList)
			}
			case protocol.PeerList: {
				n.peerList = CopyCluster(n.peerList, msg.PeerList)
				n.ConnectUnconnectedClusterPeer(n.peerList)
			}
			}
		}()
	}
}

func (n *node) StartAliveTestAck() {
	l, err := gostream.Listen(n.self.Host, protocol.NodeAliveTestProtocol)
	if err != nil {
		log.Println(err)
	}
	defer l.Close()

	for {
		conn, _ := l.Accept()
		defer conn.Close()

		go func() {
			msg := DecodeGobObjectIntoMessage(conn)
			switch msg.Operand {
			case protocol.AliveTest: {
				log.Println("接受到alivetest")
				//msg := Message{
				//	Operand: protocol.AliveTestAck,
				//}
				//log.Println("回复ack")
				//conn.Write(EncodeMessageToGobObject(msg).Bytes())
			}
			}
		}()
	}
}

// StartAliveTest 测定peer的存活, 并且用于测量bandwidth
func (n *node) StartAliveTest(ctx context.Context, clusterType int, period int) {
	// 创建一个timer设置在10s后执行
	timer := time.NewTimer(time.Second)
	switch clusterType {
	case cluster.SNList: {
		for {
			timer.Reset(time.Duration(period) * time.Second) // 复用了 timer, 每1分钟探测一次Peer的存活
			select {
			case <-timer.C: {
				for _, peer := range n.snList.Nodes {
					if peer.Id == n.self.Id {
						continue
					}
					conn, err := gostream.Dial(ctx, n.self.Host, peer.Id, protocol.NodeAliveTestProtocol)
					if err == nil {
						msg := Message{
							Operand: protocol.AliveTest,
						}
						log.Println("测定sn存活")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					} else {
						fmt.Printf("sn: Id with %s is not alive\n", peer.Id.Pretty())
						fmt.Println(err)

						// 将peer从自己的peerStore中删除
						fmt.Println("删除这个SN")
						n.self.Host.Peerstore().ClearAddrs(peer.Id)
						n.snList.RemovePeer(peer.Id)

						for _, p := range n.snList.Nodes {
							if p.Id == n.self.Id {
								continue
							}
							newConn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.UpdateClusterProtocol)
							if err != nil {
								log.Printf("fail to build updateClusterProtocol: %s\n", err)
							}

							// 将新的SnList广播给所有的SN
							peerInfoList := ConstructSendableNodesList(n.snList)

							msg := Message{
								Operand: protocol.SNList,
								SnList: cluster.Cluster{
									Id: n.snList.Id,
									Snid: n.snList.Snid,
									Nodes: peerInfoList, // nodes只有部分属性可以传过去
									Position: n.snList.Position,
								},
							}
							log.Println("发送最新的snlist")
							newConn.Write(EncodeMessageToGobObject(msg).Bytes())
						}

					}
				}
			}
			}
		}
	}
	case cluster.PeerList: {
		for {
				timer.Reset(time.Duration(period) * time.Second) // 复用了 timer, 每1分钟探测一次Peer的存活
				select {
				case <-timer.C: {
					for _, peer := range n.peerList.Nodes {
						if peer.Id == n.self.Id {
							continue
						}
						conn, err := gostream.Dial(ctx, n.self.Host, peer.Id, protocol.NodeAliveTestProtocol)
						if err == nil {
							msg := Message{
								Operand: protocol.AliveTest,
							}
							log.Println("测定peer存活")
							conn.Write(EncodeMessageToGobObject(msg).Bytes())
						} else {
							fmt.Printf("peer: Id with %s is not alive\n", peer.Id.Pretty())
							fmt.Println(err)

							// 将peer从自己的peerStore中删除
							fmt.Println("删除这个peer")
							n.self.Host.Peerstore().ClearAddrs(peer.Id)
							n.peerList.RemovePeer(peer.Id)

							for _, p := range n.peerList.Nodes {
								if p.Id == n.self.Id {
									continue
								}
								newConn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.UpdateClusterProtocol)
								if err != nil {
									log.Printf("fail to build updateClusterProtocol: %s\n", err)
								}

								// 将新的PeerList广播给所有的Normal Node
								peerInfoList := ConstructSendableNodesList(n.peerList)

								msg := Message{
									Operand: protocol.SNList,
									SnList: cluster.Cluster{
										Id: n.snList.Id,
										Snid: n.snList.Snid,
										Nodes: peerInfoList, // nodes只有部分属性可以传过去
										Position: n.snList.Position,
									},
								}
								log.Println("发送最新的peerList")
								newConn.Write(EncodeMessageToGobObject(msg).Bytes())
							}

						}
					}
				}
			}
		}
	}
	}

}

func (n *node) ConnectUnconnectedClusterPeer(c cluster.Cluster) {
	nodeSlice := n.self.Host.Peerstore().Peers()
	for _, v := range c.Nodes {
		for _, value := range nodeSlice {
			if v.Id == value {
				break
			}
		}
	}
}

func EncodeMessageToGobObject(msg Message) *bytes.Buffer {
	binBuf := new(bytes.Buffer)
	gobobj := gob.NewEncoder(binBuf)
	gobobj.Encode(msg)
	return binBuf
}

func DecodeGobObjectIntoMessage(conn net.Conn) *Message {
	tmp := make([]byte, 1024)
	_, err := conn.Read(tmp)
	if err != nil {
		fmt.Println(err)
	}
	// convert bytes into buffer
	buffer := bytes.NewBuffer(tmp)
	msg := new(Message)

	// creates a decoder obj
	gobobjdec := gob.NewDecoder(buffer)
	err = gobobjdec.Decode(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(msg)
	return msg
}

func ConstructSendableNodesList(c cluster.Cluster) []peer.Peer {
	peerInfoList := make([]peer.Peer, c.GetClusterSize())
	for index, star := range c.Nodes {
		peerInfoList[index] = peer.Peer{
			Id: star.Id,
			Mode: star.Mode,
			Position: star.Position,
			RemotePeer: star.RemotePeer,
			BandWidth: star.BandWidth,
			P2PPort: star.P2PPort,
		}
	}
	return peerInfoList
}

func CopyCluster(c1 cluster.Cluster, c2 cluster.Cluster) cluster.Cluster {
	c1.Id = c2.Id
	c1.Snid = c2.Snid
	c1.Nodes = c2.Nodes
	c1.Position = c2.Position
	return c1
}