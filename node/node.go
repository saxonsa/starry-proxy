package node

import (
	"StarryProxy/ip"
	"bufio"
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

	ConnectToNet(ctx context.Context, superNode libp2ppeer.ID)
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

type peerInfo struct {
	Pid string
	Position ip.Position
}

func (n *node) ConnectToNet(ctx context.Context, snid libp2ppeer.ID) {
	// build a stream which tags "NewNodeEntryProtocol"
	conn, _ := gostream.Dial(ctx, n.self.Host, snid, protocol.NewNodeEntryProtocol)

	// send self peer Info to supernode
	peerInfo := peerInfo{Pid: n.self.Id.Pretty(), Position: n.self.Position}
	peerInfoJson, _ := json.Marshal(peerInfo)
	fmt.Println(string(peerInfoJson))
	conn.Write(peerInfoJson)

	// recv cluster information from supernode
	reader := bufio.NewReader(conn)
	msg, _ := reader.ReadString('\n')
	fmt.Print(msg)
}

func (n *node) Serve(ctx context.Context, cfg *config.Config) {
	// start proxy service
	go n.StartProxyService()

	fmt.Println("Proxy server is ready")
	fmt.Println("libp2p-peer addresses:")
	for _, a := range n.self.Host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, libp2ppeer.Encode(n.self.Id))
	}

	if n.self.Mode == peer.SuperNode {
		// init 2 clusters
		n.snList, _ = cluster.New(n.self, cfg, cluster.SNList)
		n.peerList, _ = cluster.New(n.self, cfg, cluster.PeerList)

		// start a service waiting for the node to enter the cluster
		go n.StartNewNodeEntryService()

		// start self-proxy
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d",cfg.Proxy.Port), goproxy.NewProxyHttpServer()))
	} else if n.self.Mode == peer.NormalNode {
		n.listenOnProxy(ctx)
	}
}

func (n *node) listenOnProxy(ctx context.Context) {
	_, serveArgs, _ := manet.DialArgs(n.self.ProxyAddr)
	fmt.Println("proxy listening on ", serveArgs)
	if n.self.RemotePeer != "" {
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
	}
	<-ctx.Done()
}

func (n *node) connHandler(ctx context.Context, conn net.Conn) error {
	stream, err := n.self.Host.NewStream(ctx, n.self.RemotePeer, protocol.CommonProtocol)
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
	l, err := gostream.Listen(n.self.Host, protocol.CommonProtocol)
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
	l, err := gostream.Listen(n.self.Host, protocol.NewNodeEntryProtocol)
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
			peerInfo := peerInfo{}
			buffer := make([]byte, 1024)
			len, err := conn.Read(buffer)
			err = json.Unmarshal(buffer[:len], &peerInfo)
			if err != nil {
				log.Printf("fail to convert json to struct format: %s", err)
			}

			conn.Write([]byte("answer!\n"))
		}()
	}
}