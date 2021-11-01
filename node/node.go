package node

import (
	"StarryProxy/cluster"
	"StarryProxy/config"
	"StarryProxy/peer"
	"StarryProxy/protocol"
	"bufio"

	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/diandianl/p2p-proxy/relay"
	"github.com/elazarl/goproxy"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"

	gostream "github.com/libp2p/go-libp2p-gostream"
	manet "github.com/multiformats/go-multiaddr-net"

)

type Node interface {
	Serve(ctx context.Context, cfg *config.Config)

	ConnectToNet(superNode Node)
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

func (n *node) ConnectToNet(superNode Node) {

}

func (n *node) Serve(ctx context.Context, cfg *config.Config) {
	// start proxy service
	go func() {
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
	}()

	fmt.Println("Proxy server is ready")
	fmt.Println("libp2p-peer addresses:")
	for _, a := range n.self.Host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, libp2ppeer.Encode(n.self.Id))
	}

	if n.self.Mode == peer.SuperNode {
		// create a cluster
		n.snList, _ = cluster.New(n.self, cfg)

		// start a service waiting for the node to enter the cluster
		go func() {
			l, err := gostream.Listen(n.self.Host, protocol.NewNodeEntryProtocol)
			if err != nil {
				log.Println(err)
			}
			defer l.Close()

			for {
				conn, _ := l.Accept()
				defer conn.Close()

				reader := bufio.NewReader(conn)
				msg, _ := reader.ReadString('\n')
				fmt.Println(msg)
				conn.Write([]byte("answer!\n"))
			}
		}()

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
