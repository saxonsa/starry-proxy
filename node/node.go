package node

import (
	"StarryProxy/config"
	"StarryProxy/ip"
	"StarryProxy/protocol"
	"context"
	"fmt"
	"github.com/elazarl/goproxy"
	gostream "github.com/libp2p/go-libp2p-gostream"
	manet "github.com/multiformats/go-multiaddr-net"
	"log"
	"net"
	"net/http"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/diandianl/p2p-proxy/relay"
)

type Node interface {
	Serve(mode Mode)

	ConnectToNet(superNode Node)
}

type Mode int32

const (
	NORMAL_NODE	Mode = 0
	SUPER_NODE	Mode = 1
)

type node struct {
	Mode Mode

	Id peer.ID

	Host host.Host

	ProxyAddr ma.Multiaddr

	RemotePeer peer.ID

	Position ip.Position

	ComputingPower int
}

type listener struct {
	net.Listener
}

func New(ctx context.Context, cfg *config.Config, mode Mode) (*node, error) {
	// get node host
	h, err := makeHost(ctx, cfg)
	if err != nil {
		log.Fatalf("fail to make host: %s\n", err)
		return nil, err
	}

	// get node position
	position, err := ip.GetLocalPosition()
	if err != nil {
		log.Fatalln("Fail to get local position for node")
		return nil, err
	}

	return &node{
		Mode:     mode,
		Host:     h,
		Id: 	  h.ID(),
		Position: position,
	}, nil
}

func makeHost(ctx context.Context, cfg *config.Config) (h host.Host, err error) {
	var opt libp2p.Option
	var opts []libp2p.Option

	if opt, err = listenP2PAddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", cfg.P2P.Port)); err != nil {
		return nil, err
	}

	opts = append(opts, opt)
	h, err = libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func listenP2PAddr(addr string) (libp2p.Option, error) {
	return libp2p.ListenAddrStrings(addr), nil
}

func (n *node) ConnectToNet(superNode Node) {

}

func (n *node) Serve(ctx context.Context) {
	go func() {
		l, err := gostream.Listen(n.Host, protocol.CommonProtocol)
		if err != nil {
			log.Println(err)
		}
		proxy := goproxy.NewProxyHttpServer()
		s := &http.Server{Handler: proxy}
		err = s.Serve(l)
		if err != nil {
			return
		}
	}()

	fmt.Println("Proxy server is ready")
	fmt.Println("libp2p-peer addresses:")
	for _, a := range n.Host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, peer.Encode(n.Id))
	}

	if n.Mode == SUPER_NODE {
		<-make(chan struct{}) // hang forever
	} else if n.Mode == NORMAL_NODE {
		n.listenOnProxy(ctx)
	}
}

func (n *node) listenOnProxy(ctx context.Context) {
	fmt.Println(n.ProxyAddr)
	_, serveArgs, _ := manet.DialArgs(n.ProxyAddr)
	fmt.Println("proxy listening on ", serveArgs)
	if n.RemotePeer != "" {
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
	stream, err := n.Host.NewStream(ctx, n.RemotePeer, protocol.CommonProtocol)
	if err != nil {
		log.Fatalln(err)
	}
	err = relay.CloseAfterRelay(conn, stream)
	if err != nil {
		return err
	}
	return nil
}



