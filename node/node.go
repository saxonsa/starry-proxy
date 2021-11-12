package node

import (
	"StarryProxy/ip"
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
	P2PPort int
}

type Message struct {
	Operand int
	SnList cluster.Cluster
	ExistedSupernode peer.Peer // 如果访问的peer所在的区域已经有supernode, 将supernode的信息发给peer
}

func (n *node) ConnectToNet(ctx context.Context, cfg *config.Config, snid libp2ppeer.ID) {
	// The first node entered the p2p net
	if n.self.Mode == peer.SSPNode {
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
	peerInfo := peerInfo{PeerAddr: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s", cfg.P2P.Port, n.self.Id),
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
		// create a temp buffer
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

		switch msg.Operand {
			case protocol.EXIT: {
				return
			}
			case protocol.AssignSelfAsSupernode: {
				go n.StartNewNodeEntryService(cfg)
				n.peerList, _ = cluster.New(n.self, cfg, cluster.PeerList)

				// copy msg.Snlist into n.snlist
				n.snList.Id = msg.SnList.Id
				n.snList.Snid = msg.SnList.Snid
				n.snList.Position = msg.SnList.Position
				n.snList.Nodes = msg.SnList.Nodes
			}
			case protocol.ExistedSupernodeInSelfCluster: {
				// 获取supernode的peer信息
				fmt.Printf("msg uid: %s\n", msg.ExistedSupernode.Id)
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

func (n *node) Serve(ctx context.Context, cfg *config.Config) {

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

	if n.self.Mode == peer.SuperNode || n.self.Mode == peer.SSPNode {
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
				log.Printf("fail to convert json to struct format: %s", err)
			}

			// put the normal node in the right cluster
			if pInfo.Position == n.peerList.GetClusterPosition() {
				fmt.Println("put the node in my cluster")
				// at the same position - put in the same cluster - conn directly to sn
				err := n.peerList.AddPeer(peer.Peer{
					Id: libp2ppeer.ID(pInfo.PeerAddr),
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
						P2PPort: pInfo.P2PPort,
					})

					// 让peer将自己作为supernode

					// 将snlist的信息传给这个peer
					peerInfoList := make([]peer.Peer, n.snList.GetClusterSize())
					for index, star := range n.snList.Nodes {
						peerInfoList[index] = peer.Peer{
							Id: star.Id,
							Mode: star.Mode,
							Position: star.Position,
							RemotePeer: star.RemotePeer,
							BandWidth: star.BandWidth,
							P2PPort: star.P2PPort,
						}
					}

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

func EncodeMessageToGobObject(msg Message) *bytes.Buffer {
	binBuf := new(bytes.Buffer)
	gobobj := gob.NewEncoder(binBuf)
	gobobj.Encode(msg)
	return binBuf
}