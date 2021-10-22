package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	manet "github.com/multiformats/go-multiaddr-net"
	"log"
	"net"
	"net/http"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/elazarl/goproxy"

	gostream "github.com/libp2p/go-libp2p-gostream"

	"github.com/diandianl/p2p-proxy/relay"
)

const Protocol = "/proxy-example/0.0.1"

type listener struct {
	net.Listener
}

// makeRandomHost creates a libp2p host with a randomly generated identity.
// This step is described in depth in other tutorials.
func makeRandomHost(ctx context.Context, port int) host.Host {
	host, err := libp2p.New(ctx, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port)))
	if err != nil {
		log.Fatalln(err)
	}
	return host
}

// ProxyService provides HTTP proxying on top of libp2p by launching an
// HTTP server which tunnels the requests to a destination peer running
// ProxyService too.
type ProxyService struct {
	host      host.Host
	dest      peer.ID
	proxyAddr ma.Multiaddr
}

// NewProxyService attaches a proxy service to the given libp2p Host.
// The proxyAddr parameter specifies the address on which the
// HTTP proxy server listens. The dest parameter specifies the peer
// ID of the remote peer in charge of performing the HTTP requests.
//
// ProxyAddr/dest may be nil/"" it is not necessary that this host
// provides a listening HTTP server (and instead its only function is to
// perform the proxied http requests it receives from a different peer.
//
// The addresses for the dest peer should be part of the host's peerstore.
func NewProxyService(h host.Host, proxyAddr ma.Multiaddr, dest peer.ID) *ProxyService {
	// We let our host know that it needs to handle streams tagged with the
	// protocol id that we have defined, and then handle them to
	// our own streamHandling function.
	//h.SetStreamHandler(Protocol, streamHandler)
	go func() {
		l, err := gostream.Listen(h, Protocol)
		if err != nil {
			log.Println(err)
		}
		proxy := goproxy.NewProxyHttpServer()
		s := &http.Server{Handler: proxy}
		s.Serve(l)
	}()

	fmt.Println("Proxy server is ready")
	fmt.Println("libp2p-peer addresses:")
	for _, a := range h.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", a, peer.Encode(h.ID()))
	}

	return &ProxyService{
		host:      h,
		dest:      dest,
		proxyAddr: proxyAddr,
	}
}


// Serve listens on the ProxyService's proxy address. This effectively
// allows to set the listening address as http proxy.
func (p *ProxyService) Serve(ctx context.Context) {
	_, serveArgs, _ := manet.DialArgs(p.proxyAddr)
	fmt.Println("proxy listening on ", serveArgs)
	if p.dest != "" {
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
				go p.connHandler(ctx, conn)
			}
		}()
	}
	<-ctx.Done()
}

func (p *ProxyService) connHandler(ctx context.Context, conn net.Conn) error {
	stream, err := p.host.NewStream(ctx, p.dest, Protocol)
	if err != nil {
		log.Fatalln(err)
	}
	relay.CloseAfterRelay(conn, stream)
	return nil
}

// addAddrToPeerstore parses a peer multiaddress and adds
// it to the given host's peerstore, so it knows how to
// contact it. It returns the peer ID of the remote peer.
func addAddrToPeerstore(h host.Host, addr string) peer.ID {
	// The following code extracts target's the peer ID from the
	// given multiaddress
	ipfsaddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		log.Fatalln(err)
	}
	pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		log.Fatalln(err)
	}

	peerid, err := peer.Decode(pid)
	if err != nil {
		log.Fatalln(err)
	}

	// Decapsulate the /ipfs/<peerID> part from the target
	// /ip4/<a.b.c.d>/ipfs/<peer> becomes /ip4/<a.b.c.d>
	targetPeerAddr, _ := ma.NewMultiaddr(
		fmt.Sprintf("/ipfs/%s", peer.Encode(peerid)))
	targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

	// We have a peer ID and a targetAddr so we add
	// it to the peerstore so LibP2P knows how to contact it
	h.Peerstore().AddAddr(peerid, targetAddr, peerstore.PermanentAddrTTL)
	return peerid
}

func main() {
	ctx := context.Background()

	destPeer := flag.String("d", "", "destination peer address")
	port := flag.Int("p", 9900, "proxy port")
	p2pport := flag.Int("l", 12000, "libp2p listen port")
	flag.Parse()

	// If we have a destination peer we will start a local server
	if *destPeer != "" {
		// We use p2pport+1 in order to not collide if the user
		// is running the remote peer locally on that port
		host := makeRandomHost(ctx, *p2pport + 1)
		// Make sure our host knows how to reach destPeer
		destPeerID := addAddrToPeerstore(host, *destPeer)
		proxyAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", *port))
		if err != nil {
			log.Fatalln(err)
		}
		// Create the proxy service and start the http server
		proxy := NewProxyService(host, proxyAddr, destPeerID)
		proxy.Serve(ctx) // serve hangs forever
	} else {
		host := makeRandomHost(ctx, *p2pport)
		// In this case we only need to make sure our host
		// knows how to handle incoming proxied requests from
		// another peer.
		_ = NewProxyService(host, nil, "")
		<-make(chan struct{}) // hang forever
	}
}