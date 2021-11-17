package node

import (
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
	"StarryProxy/ip"
	"StarryProxy/parameters"
	"StarryProxy/peer"
	"StarryProxy/protocol"
	"StarryProxy/request"

	"github.com/diandianl/p2p-proxy/relay"
	"github.com/elazarl/goproxy"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	gostream "github.com/libp2p/go-libp2p-gostream"
	manet "github.com/multiformats/go-multiaddr-net"
)

type Node interface {
	Serve(ctx context.Context, params *parameters.Parameter)

	ConnectToNet(ctx context.Context, cfg *config.Config, superNode libp2ppeer.ID)
}

type node struct {
	self peer.Peer

	peerList cluster.Cluster

	snList	cluster.Cluster

	peerListTimer time.Timer

	snListTimer time.Timer
}


type listener struct {
	net.Listener
}

func New(peer peer.Peer) (Node, error) {
	gob.Register(Message{})
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
	Rate float64
}

type Message struct {
	Operand int
	SnList cluster.Cluster
	PeerList cluster.Cluster
	ExistedSupernode peer.Peer // 如果访问的peer所在的区域已经有supernode, 将supernode的信息发给peer
	Sender peer.Peer
	NewSN peer.Peer
	ClusterType int
	NewNode peer.Peer
	LeaveNode peer.Peer
}

func (n *node) ConnectToNet(ctx context.Context, cfg *config.Config, snid libp2ppeer.ID) {
	// The first node entered the p2p net
	if n.self.Mode == peer.MasterNode {
		fmt.Println("enter p2p net test")

		if cfg.Demo {
			params := make(map[string]string)
			params["mode"] = string(rune(peer.MasterNode))
			params["peer_id"] = string(n.self.Id)
			params["peer_name"] = cfg.Name
			params["province"] = n.self.Position.Province
			params["city"] = n.self.Position.City
			request.Post("/enter_p2p_net", params)
		}

		// init 2 clusters
		n.snList, _ = cluster.New(n.self, cfg)
		n.peerList, _ = cluster.New(n.self, cfg)
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
		Position: n.self.Position, P2PPort: cfg.P2P.Port, Rate: n.self.Rate}
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
				log.Println("接收clusterList")
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
				n.self.Mode = peer.SuperNode
				n.peerList, _ = cluster.New(n.self, cfg)

				// copy msg.Snlist into n.snlist
				n.snList = CopyCluster(n.snList, msg.SnList)

				// 将snlist中和自己没有连接起来的连起来
				n.ConnectUnconnectedClusterPeer(n.snList)
			}
			case protocol.ExistedSupernodeInSelfCluster: {
				log.Println("get a new SN peer info")

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

func (n *node) Serve(ctx context.Context, params *parameters.Parameter) {

	// 随机一个remote peer
	remotePeer := n.peerList.FindRandomPeer(n.self.Id)
	if remotePeer != nil {
		n.self.RemotePeer = remotePeer.Id
	}

	// do something when node quit
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		// TODO: peer退出逻辑
		log.Printf("test::;.......n.self.Mode: %d\n", n.self.Mode)
		switch n.self.Mode {
		case peer.NormalNode:
			{
				for _, p := range n.peerList.Nodes {
					if p.Id == n.self.Id { // 不能给自己发
						continue
					}
					conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
					if err == nil {
						msg := Message{
							Operand: protocol.NodeLeave,
							Sender: peer.Peer{
								Id: n.self.Id,
								Mode: peer.NormalNode,
							},
						}
						log.Println("告诉peerList中的node自己离开")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					}
				}
			}
		case peer.SuperNode:
			{
				backup := peer.Peer{Id: "", P2PPort: 0}
				// 如果cluster里面只有supernode
				if n.peerList.GetClusterSize() != 1 {
					// 将权限交给backup
					conn, err := gostream.Dial(ctx, n.self.Host, n.peerList.Backup.Id, protocol.CommonManageProtocol)
					if err != nil {
						// backup不存在
						log.Printf("fail to dial backup while exit: %s\n", err)
					} else {
						// backup 存在
						log.Println("让backup成为SN")
						backup = peer.Peer{Id: n.peerList.Backup.Id, P2PPort: n.peerList.Backup.P2PPort}
						msg := Message{
							Operand: protocol.AssignSelfAsSupernode,
							ClusterType: cluster.PeerList,
							Sender: peer.Peer{
								Id: n.self.Id,
								Mode: peer.SuperNode,
							},
							LeaveNode: peer.Peer{
								Id: n.self.Id,
								Mode: peer.SuperNode,
							},
						}
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					}
				}

				// 告诉所有的SN自己的离开
				for _, p := range n.snList.Nodes {
					if p.Id == n.self.Id {
						continue
					}
					conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
					if err == nil {
						msg := Message{
							Operand: protocol.NodeLeave,
							Sender: peer.Peer{
								Id: n.self.Id,
								Mode: peer.SuperNode,
							},
							NewNode: peer.Peer{
								Id: backup.Id,
								P2PPort: backup.P2PPort,
							},
						}
						log.Println("告诉snlist中的node自己离开")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					}
				}

				// 向PeerList广播自己的离开
				for _, p := range n.peerList.Nodes {
					if p.Id == n.self.Id || p.Id == n.peerList.Backup.Id { // 不再向Backup发了
						continue
					}
					conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
					if err == nil {
						msg := Message{
							Operand: protocol.NodeLeave,
							Sender: peer.Peer{
								Id: n.self.Id,
								Mode: peer.SuperNode,
							},
							NewSN: peer.Peer{
								Id: n.peerList.Backup.Id,
								Mode: peer.SuperNode,
								P2PPort: n.peerList.Backup.P2PPort,
								Position: n.peerList.Backup.Position,
							},
						}
						log.Println("SN 告诉peerList中的node自己离开")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					}
				}
			}
		case peer.MasterNode:
			{
				if n.peerList.GetClusterSize() > 1 {
					backup := peer.Peer{Id: "", P2PPort: 0}
					// 让backup作为SN
					conn, err := gostream.Dial(ctx, n.self.Host, n.peerList.Backup.Id, protocol.CommonManageProtocol)
					if err != nil {
						fmt.Printf("fail to assign the second node as SN: %s\n", err)
					} else {
						backup = peer.Peer{Id: n.peerList.Backup.Id, P2PPort: n.peerList.Backup.P2PPort}
						msg := Message{
							Operand: protocol.AssignSelfAsSupernode,
							ClusterType: cluster.PeerList,
							LeaveNode: peer.Peer{Id: n.self.Id, P2PPort: n.self.P2PPort},
						}
						log.Println("assign backup as SN")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					}


					// 向peerList广播自己的离开
					for _, p := range n.peerList.Nodes {
						if p.Id == n.self.Id {
							continue
						}
						conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
						if err == nil {
							msg := Message{
								Operand: protocol.NodeLeave,
								Sender: peer.Peer{
									Id: n.self.Id,
									Mode: peer.MasterNode,
								},
								NewSN: peer.Peer{
									Id: backup.Id,
									P2PPort: backup.P2PPort,
								},
							}
							log.Println("告诉peerList中的node自己离开")
							conn.Write(EncodeMessageToGobObject(msg).Bytes())
						}
					}
				}

				if n.snList.GetClusterSize() > 1 {
					backup := peer.Peer{Id: "", P2PPort: 0}

					// 让backup作为Master
					conn, err := gostream.Dial(ctx, n.self.Host, n.snList.Backup.Id, protocol.CommonManageProtocol)
					if err != nil {
						fmt.Printf("fail to assign Backup SN as Master node: %s\n", err)
					} else {
						backup = peer.Peer{Id: n.snList.Backup.Id, P2PPort: n.snList.Backup.P2PPort}
						msg := Message{
							Operand: protocol.AssignSelfAsSupernode,
							ClusterType: cluster.SNList,
							LeaveNode: peer.Peer{Id: n.self.Id, P2PPort: n.self.P2PPort},
						}
						log.Println("assign backup as master")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					}


					// 告诉所有的SN自己的离开
					for _, p := range n.snList.Nodes {
						if p.Id == n.self.Id {
							continue
						}
						conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
						if err == nil {
							msg := Message{
								Operand: protocol.NodeLeave,
								Sender: peer.Peer{
									Id: n.self.Id,
									Mode: peer.MasterNode,
								},
								NewSN: peer.Peer{
									Id: backup.Id,
									P2PPort: backup.P2PPort,
								},
							}
							log.Println("告诉snlist中的node自己离开")
							conn.Write(EncodeMessageToGobObject(msg).Bytes())
						}
					}
				}
			}
		}
		fmt.Println("node quit!")
		os.Exit(0)
	}()

	// 启动 CommonManageProtocol, 更新SNList, PeerList, 接收心跳测试
	go n.StartService(ctx, params)

	// 启动proxy service, 监听 gostream <commonProtocol>, 将收到的http请求用goproxy处理掉
	go n.StartProxyService()

	// 处理新的node加入
	go n.StartNewNodeEntryService()

	fmt.Println("libp2p-peer addresses:")
	for _, a := range n.self.Host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, libp2ppeer.Encode(n.self.Id))
	}

	switch n.self.Mode {
		case peer.MasterNode: {
			go n.StartAliveTest(ctx, cluster.SNList, params.HeartBeat.SnList)
			go n.StartAliveTest(ctx, cluster.PeerList, params.HeartBeat.PeerList)
		}
		case peer.SuperNode: {
			go n.StartAliveTest(ctx, cluster.PeerList, params.HeartBeat.PeerList)
		}
	}

	//go n.TestHeartBeat(params)

	// 监听设置好的proxy端口, 将http请求转发到这个端口上, 然后端口将stream转发给remote proxy
	// 如果没有remote peer, 自己处理端口的请求
	n.listenOnProxy(ctx)
}

//func (n *node) TestHeartBeat(params *parameters.Parameter) {
//	timer := time.NewTimer(time.Second)
//	for {
//		timer.Reset(time.Duration(params.HeartBeat.PeerList) * time.Second) // 复用了 timer, 每1分钟探测一次Peer的存活
//		select {
//		case <-timer.C: {
//
//		}}
//	}
//}


// StartService 启动 CommonManageProtocol, 更新SNList, PeerList, 接收心跳测试
func (n *node) StartService(ctx context.Context, params *parameters.Parameter) {
	l, err := gostream.Listen(n.self.Host, protocol.CommonManageProtocol)
	if err != nil {
		log.Println(err)
	}
	defer l.Close()
	for {
		conn, _ := l.Accept()
		go func() {
			msg := DecodeGobObjectIntoMessage(conn)
			switch msg.Operand {
			case protocol.UpdateSNList: { // 接收更新的SNList
				n.snList = CopyCluster(n.snList, msg.SnList)

				// 将snlist中和自己没有连接起来的连起来
				n.ConnectUnconnectedClusterPeer(n.snList)
			}
			case protocol.UpdatePeerList: { // 接收更新的PeerList
				n.peerList = CopyCluster(n.peerList, msg.PeerList)
				n.ConnectUnconnectedClusterPeer(n.peerList)
			}
			case protocol.AliveTest: { // 接收心跳测试
				log.Println("接收到alivetest")
				//msg := Message{
				//	Operand: protocol.AliveTestAck,
				//}
				//log.Println("回复ack")
				//conn.Write(EncodeMessageToGobObject(msg).Bytes())
			}
			case protocol.AssignSelfAsSupernode: {
				if msg.ClusterType == cluster.PeerList {
					log.Println("成为SN")
					n.self.Mode = peer.SuperNode
					if msg.LeaveNode.Id != "" {
						n.peerList.RemovePeer(msg.LeaveNode.Id)
					}
					n.peerList.SN = n.self
					go n.StartAliveTest(ctx, cluster.PeerList, params.HeartBeat.PeerList)

				} else { // SSN
					n.self.Mode = peer.MasterNode
					log.Println("成为Master")
					if msg.LeaveNode.Id != "" {
						n.snList.RemovePeer(msg.LeaveNode.Id)
					}
					n.snList.SN = n.self
					go n.StartAliveTest(ctx, cluster.SNList, params.HeartBeat.SnList)
					go n.StartAliveTest(ctx, cluster.PeerList, params.HeartBeat.PeerList)
				}
			}

			// --------------------------- Backup protocol ------------------------------------------------------
			case protocol.RaiseSnListBackup: {
				n.snList.Backup = n.self
				// 启动backup服务
				n.BackupService(true, cluster.SNList)
			}
			case protocol.OffSnListBackup: {
				// 取消backup服务
				n.BackupService(false, cluster.SNList)
			}
			case protocol.ChangeSnListBackup: {
				n.snList.Backup = msg.SnList.Backup
			}
			case protocol.RaisePeerListBackup: {
				n.peerList.Backup = n.self
				// 启动backup服务
				n.BackupService(true, cluster.PeerList)
			}
			case protocol.OffPeerListBackup: {
				// 取消backup服务
				n.BackupService(false, cluster.PeerList)
			}
			case protocol.ChangePeerListBackup: {
				n.peerList.Backup = msg.PeerList.Backup
			}
			// --------------------------------------------------------------------------------------------------

			case protocol.NodeLeave: {
				n.self.Host.Peerstore().ClearAddrs(msg.Sender.Id)
				switch msg.Sender.Mode {
				case peer.NormalNode: {
					log.Println("PeerList中有normal node离开, 更新自己的peerList")
					n.peerList.RemovePeer(msg.Sender.Id)
				}
				case peer.SuperNode: {
					if n.self.Mode == peer.SuperNode || n.self.Mode == peer.MasterNode {


						// 将sn从自己的snList中删除
						log.Println("删除要退出的peer")
						n.snList.RemovePeer(msg.Sender.Id)

						// 如果有新的backup成为了sn, 在自己的snList上加入新的node
						if msg.NewNode.Id != "" {
							err := n.snList.AddPeer(msg.NewNode)
							if err != nil {
								return
							}
							peer.AddAddrToPeerstore(n.self.Host, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", msg.NewNode.P2PPort, msg.NewNode.Id))
						}

						// 广播给所有的peerList
						log.Println("向PeerList中的peer广播最新的peerList")
						for _, p := range n.peerList.Nodes {
							if p.Id == n.self.Id { // 不广播自己
								continue
							}

							conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
							if err != nil {
								fmt.Printf("fail to broadcast peerList: %s\n", err)
							}

							SNInfoList :=  ConstructSendableNodesList(n.snList)
							msg := Message{
								Operand: protocol.UpdateSNList,
								SnList: cluster.Cluster{
									Id: n.snList.Id,
									SN: peer.Peer{Id: n.snList.SN.Id, P2PPort: n.snList.SN.P2PPort},
									Nodes: SNInfoList,
									Position: n.snList.Position,
								},
							}
							conn.Write(EncodeMessageToGobObject(msg).Bytes())
						}

					} else { // normal node
						n.peerList.RemovePeer(msg.Sender.Id)

						// 切换SN
						n.peerList.SN = peer.Peer{Id: msg.NewSN.Id, P2PPort: msg.NewSN.P2PPort}
						peer.AddAddrToPeerstore(n.self.Host, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s",
							msg.NewSN.P2PPort, msg.NewSN.Id))
					}
				}
				case peer.MasterNode: {

					// 将sn从自己的snList中删除
					log.Println("删除masterNode")
					n.snList.RemovePeer(msg.Sender.Id)

					if n.self.Mode == peer.SuperNode {
						// 如果有新的backup成为了Master - 加入到snList里面、
						if msg.NewSN.Id != "" {
							err := n.snList.AddPeer(msg.NewSN)
							if err != nil {
								return
							}
							peer.AddAddrToPeerstore(n.self.Host, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", msg.NewSN.P2PPort, msg.NewSN.Id))
						}

						// 广播给所有的peerList
						for _, p := range n.peerList.Nodes {
							if p.Id == n.self.Id {
								continue
							}

							conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
							if err != nil {
								fmt.Printf("fail to broadcast peerList: %s\n", err)
							}

							SNInfoList :=  ConstructSendableNodesList(n.snList)
							msg := Message{
								Operand: protocol.UpdateSNList,
								SnList: cluster.Cluster{
									Id: n.snList.Id,
									Backup: n.snList.Backup,
									SN: peer.Peer{Id: msg.SnList.SN.Id, P2PPort: msg.SnList.SN.P2PPort},
									Nodes: SNInfoList,
									Position: n.snList.Position,
								},
							}
							conn.Write(EncodeMessageToGobObject(msg).Bytes())
						}

					} else { // normal node
						// 如果有新的backup成为了Supernode - 加入到peerList里面
						if msg.NewSN.Id != "" {
							err := n.peerList.AddPeer(msg.NewSN)
							if err != nil {
								return
							}
							peer.AddAddrToPeerstore(n.self.Host, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", msg.NewSN.P2PPort, msg.NewSN.Id))

							// 连接到新的Supernode
							n.peerList.SN = peer.Peer{Id: msg.NewSN.Id, P2PPort: msg.NewSN.P2PPort}
						}
					}
				}
				}
			}
			}
		}()
	}
}

func (n *node) BackupService(on bool, clusterType int) {
	switch clusterType {
	case cluster.SNList: {
		if on {
			log.Println("开启SNList backup服务")
			n.self.Service.SnListBackup = true
		} else {
			log.Println("关闭SNList backup服务")
			n.self.Service.SnListBackup = false
		}
	}
	case cluster.PeerList: {
		if on {
			log.Println("开启peerList backup服务")
			n.self.Service.PeerListBackup = true
		} else {
			log.Println("关闭peerList backup服务")
			n.self.Service.PeerListBackup = false
		}
	}
	}
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
			buffer := make([]byte, 2048)
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
					Rate: pInfo.Rate,
				})
				if err != nil {
					fmt.Printf("fail to add a peer to peerList: %s", err)
				}
				peer.AddAddrToPeerstore(n.self.Host, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", pInfo.P2PPort, pInfo.Id))

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
						SN: peer.Peer{Id: n.peerList.SN.Id, P2PPort: n.peerList.SN.P2PPort},
						Backup: peer.Peer{Id: ""},
						Nodes: peerInfoList,
						Position: n.peerList.Position,
					},
					SnList: cluster.Cluster{
						Id: n.snList.Id,
						SN: peer.Peer{Id: n.snList.SN.Id, P2PPort: n.snList.SN.P2PPort},
						Backup: peer.Peer{Id: ""},
						Nodes: SNInfoList,
						Position: n.snList.Position,
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
						Rate: pInfo.Rate,
					})
					peer.AddAddrToPeerstore(n.self.Host, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", pInfo.P2PPort, pInfo.Id))

					// 将snlist的信息传给这个peer
					peerInfoList := ConstructSendableNodesList(n.snList)

					msg := Message{
						Operand: protocol.AssignSelfAsSupernode,
						SnList: cluster.Cluster{
							Id: n.snList.Id,
							SN: peer.Peer{Id: n.snList.SN.Id, P2PPort: n.snList.SN.P2PPort},
							Nodes: peerInfoList, // nodes只有部分属性可以传过去
							Position: n.snList.Position,
						},
						LeaveNode: peer.Peer{Id: ""},
					}
					conn.Write(EncodeMessageToGobObject(msg).Bytes())
				} else {
					// found a supernode in current peer's position(cluster)
					log.Println("found a SN in current peer's position")
					msg := Message{Operand: protocol.ExistedSupernodeInSelfCluster, ExistedSupernode: peer.Peer{
						Id: p.Id,
						P2PPort: p.P2PPort,
						Mode: p.Mode,
					}}
					conn.Write(EncodeMessageToGobObject(msg).Bytes())

					// 将这个peer从自己的ps中删除
					n.self.Host.Peerstore().ClearAddrs(pInfo.Id)

					return
				}
			}
			msg := Message{Operand: protocol.EXIT}
			conn.Write(EncodeMessageToGobObject(msg).Bytes())
		}()
	}
}

// StartAliveTest 测定peer的存活, 并且用于测量bandwidth和更新cluster
func (n *node) StartAliveTest(ctx context.Context, clusterType int, period int) {
	// 创建一个timer设置在10s后执行
	timer := time.NewTimer(time.Second)
	switch clusterType {
	case cluster.SNList: {
		for {
			timer.Reset(time.Duration(period) * time.Second) // 复用了 timer, 每1分钟探测一次Peer的存活
			select {
			case <-timer.C: {
				for _, p := range n.snList.Nodes {
					if p.Id == n.self.Id {
						continue
					}
					conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
					if err == nil {
						msg := Message{
							Operand: protocol.AliveTest,
						}
						log.Println("测定sn存活")
						conn.Write(EncodeMessageToGobObject(msg).Bytes())
					} else {
						fmt.Printf("sn: Id with %s is not alive\n", p.Id.Pretty())
						fmt.Println(err)

						// 将peer从自己的peerStore中删除
						fmt.Println("删除这个SN")
						n.self.Host.Peerstore().ClearAddrs(p.Id)
						n.snList.RemovePeer(p.Id)

						for _, p := range n.snList.Nodes {
							if p.Id == n.self.Id {
								continue
							}
							newConn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
							if err != nil {
								log.Printf("fail to build updateClusterProtocol: %s\n", err)
							}

							// 将新的SnList广播给所有的SN
							peerInfoList := ConstructSendableNodesList(n.snList)

							msg := Message{
								Operand: protocol.UpdateSNList,
								SnList: cluster.Cluster{
									Id: n.snList.Id,
									SN: peer.Peer{Id: n.snList.SN.Id, P2PPort: n.snList.SN.P2PPort},
									Nodes: peerInfoList, // nodes只有部分属性可以传过去
									Position: n.snList.Position,
								},
							}
							log.Println("发送最新的snlist")
							newConn.Write(EncodeMessageToGobObject(msg).Bytes())
						}

					}
				}

				secondRankPeer, str := n.snList.FindSecondRankPeer()
				if str == "" {
					// 如果现在的选出来的backup和原来的相同 - 不改变
					// 如果不相同 - 告诉原来的backup不成为backup, 并告诉新的backup成为backup
					if n.snList.Backup.Id == "" { // 此时没有backup

						// 告诉这个peer成为backup
						conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
						if err != nil {
							log.Printf("fail to dial backup: %s\n", err)
						}
						n.snList.Backup = peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort}
						msg := Message{
							Operand: protocol.RaiseSnListBackup,
						}
						conn.Write(EncodeMessageToGobObject(msg).Bytes())

						// 广播给所有人新的backup
						for _, p := range n.snList.Nodes {
							if p.Id != n.self.Id && p.Id != secondRankPeer.Id {
								conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
								if err != nil {
									log.Printf("fail to dial peer for backup: %s\n", err)
								}
								msg := Message{
									Operand: protocol.ChangeSnListBackup,
									SnList: cluster.Cluster{
										Backup: peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort},
									},
								}
								conn.Write(EncodeMessageToGobObject(msg).Bytes())
							}
						}
					}
					if secondRankPeer.Id != n.snList.Backup.Id {
						// 如果有backup并且新的和原来的不一样　- 更新backup

						// 告诉原来的backup不成为backup
						OriConn, err := gostream.Dial(ctx, n.self.Host, n.snList.Backup.Id, protocol.CommonManageProtocol)
						if err != nil {
							log.Printf("fail to dial backup: %s\n", err)
						} else {
							msg := Message{
								Operand: protocol.OffSnListBackup,
							}
							OriConn.Write(EncodeMessageToGobObject(msg).Bytes())
						}

						// 告诉新的backup成为backup
						conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
						if err != nil {
							log.Printf("fail to dial backup: %s\n", err)
						}
						n.snList.Backup = peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort}
						msg := Message{
							Operand: protocol.RaiseSnListBackup,
						}
						conn.Write(EncodeMessageToGobObject(msg).Bytes())

						// 告诉snList中的人新的backup
						for _, p := range n.snList.Nodes {
							if p.Id != n.self.Id && p.Id != secondRankPeer.Id {
								conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
								if err != nil {
									log.Printf("fail to dial peer for backup: %s\n", err)
								}
								msg := Message{
									Operand: protocol.ChangeSnListBackup,
									SnList: cluster.Cluster{
										Backup: peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort},
									},
								}
								conn.Write(EncodeMessageToGobObject(msg).Bytes())
							}
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
					for _, p := range n.peerList.Nodes {
						if p.Id == n.self.Id {
							continue
						}
						conn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
						if err == nil {
							msg := Message{
								Operand: protocol.AliveTest,
							}
							log.Println("测定peer存活")
							conn.Write(EncodeMessageToGobObject(msg).Bytes())
						} else {
							fmt.Printf("peer: Id with %s is not alive\n", p.Id.Pretty())
							fmt.Println(err)

							// 将peer从自己的peerStore中删除
							fmt.Println("删除这个peer")
							n.self.Host.Peerstore().ClearAddrs(p.Id)
							n.peerList.RemovePeer(p.Id)

							for _, p := range n.peerList.Nodes {
								if p.Id == n.self.Id {
									continue
								}
								newConn, err := gostream.Dial(ctx, n.self.Host, p.Id, protocol.CommonManageProtocol)
								if err != nil {
									log.Printf("fail to build updateClusterProtocol: %s\n", err)
								}

								// 将新的PeerList广播给所有的Normal Node
								peerInfoList := ConstructSendableNodesList(n.peerList)

								msg := Message{
									Operand: protocol.UpdatePeerList,
									PeerList: cluster.Cluster{
										Id: n.peerList.Id,
										SN: peer.Peer{Id: n.peerList.SN.Id, P2PPort: n.peerList.SN.P2PPort},
										Nodes: peerInfoList, // nodes只有部分属性可以传过去
										Position: n.peerList.Position,
									},
								}
								log.Println("发送最新的peerList")
								newConn.Write(EncodeMessageToGobObject(msg).Bytes())
							}

						}
					}

					secondRankPeer, str := n.peerList.FindSecondRankPeer()
					if str == "" {
						// 如果现在的选出来的backup和原来的相同 - 不改变
						// 如果不相同 - 告诉原来的backup不成为backup, 并告诉新的backup成为backup
						if n.peerList.Backup.Id == "" { // 此时没有backup

							// 告诉这个peer成为backup
							conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
							if err != nil {
								log.Printf("fail to dial backup: %s\n", err)
							}
							n.peerList.Backup = peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort}
							msg := Message{
								Operand: protocol.RaisePeerListBackup,
							}
							conn.Write(EncodeMessageToGobObject(msg).Bytes())

							// 广播给peerList中所有人新的backup
							for _, p := range n.peerList.Nodes {
								if p.Id != n.self.Id && p.Id != secondRankPeer.Id {
									conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
									if err != nil {
										log.Printf("fail to dial peer for backup: %s\n", err)
									}
									msg := Message{
										Operand: protocol.ChangePeerListBackup,
										PeerList: cluster.Cluster{
											Backup: peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort},
										},
									}
									conn.Write(EncodeMessageToGobObject(msg).Bytes())
								}
							}
						}
						if secondRankPeer.Id != n.peerList.Backup.Id {
							// 如果有backup并且新的和原来的不一样　- 更新backup

							// 如果原来的backup没有下线 - 告诉原来的backup不成为backup
							OriConn, err := gostream.Dial(ctx, n.self.Host, n.peerList.Backup.Id, protocol.CommonManageProtocol)
							if err != nil { // 下线
								log.Printf("backup offline already: %s\n", err)
							} else { // 在线
								msg := Message{
									Operand: protocol.OffPeerListBackup,
								}
								OriConn.Write(EncodeMessageToGobObject(msg).Bytes())
							}

							// 告诉新的backup成为backup
							conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
							if err != nil {
								log.Printf("fail to dial backup: %s\n", err)
							}
							n.peerList.Backup = peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort}
							msg := Message{
								Operand: protocol.RaisePeerListBackup,
							}
							conn.Write(EncodeMessageToGobObject(msg).Bytes())

							// 告诉peerList中的人新的backup
							for _, p := range n.peerList.Nodes {
								if p.Id != n.self.Id && p.Id != secondRankPeer.Id {
									conn, err := gostream.Dial(ctx, n.self.Host, secondRankPeer.Id, protocol.CommonManageProtocol)
									if err != nil {
										log.Printf("fail to dial peer for backup: %s\n", err)
									}
									msg := Message{
										Operand: protocol.ChangePeerListBackup,
										PeerList: cluster.Cluster{
											Backup: peer.Peer{Id: secondRankPeer.Id, P2PPort: secondRankPeer.P2PPort},
										},
									}
									conn.Write(EncodeMessageToGobObject(msg).Bytes())
								}
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
		flag := true
		for _, value := range nodeSlice {
			if v.Id == value {
				flag = false
				break
			}
		}
		if flag {
			n.self.Host.Peerstore().Addrs(v.Id)
		}
	}
}

//func (n *node) DeleteNonExistPeer(c cluster.Cluster) {
//	nodeSlice := n.self.Host.Peerstore().Peers() // 多
//	for _, v := range nodeSlice {
//		flag := false
//		for _, value := range c.Nodes {
//			// 如果在里面
//			if v == value.Id {
//				flag = true
//				break
//			}
//		}
//		if !flag {
//			// 删除
//			n.self.Host.Peerstore().ClearAddrs(v)
//		}
//	}
//}

func EncodeMessageToGobObject(msg Message) *bytes.Buffer {
	binBuf := new(bytes.Buffer)
	gobobj := gob.NewEncoder(binBuf)
	gobobj.Encode(msg)
	return binBuf
}

func DecodeGobObjectIntoMessage(conn net.Conn) *Message {
	tmp := make([]byte, 2048)
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
			//Position: star.Position,
			RemotePeer: star.RemotePeer,
			P2PPort: star.P2PPort,
		}
	}
	return peerInfoList
}

func CopyCluster(c1 cluster.Cluster, c2 cluster.Cluster) cluster.Cluster {
	c1.Id = c2.Id
	c1.SN = peer.Peer{Id: c2.SN.Id, P2PPort: c2.SN.P2PPort}
	c1.Nodes = c2.Nodes
	c1.Position = c2.Position
	return c1
}