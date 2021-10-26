# StarryProxy

<p align="center">
  <img src="https://img.shields.io/badge/made%20by-Saxon SA%20&%20Fay HU-blue.svg?style=flat-square" />
  <img src="https://img.shields.io/badge/project-StarryProxy-yellow.svg?style=flat-square" />
<img src="https://img.shields.io/badge/version-0.0.1-darkgreen.svg?style=flat-square" />
</p>

a proxy based on [`libp2p`](https://github.com/libp2p/go-libp2p) and [`goproxy`](https://github.com/elazarl/goproxy)

Part of inspiration comes from [`p2p-proxy`](https://github.com/diandianl/p2p-proxy)


Create your own `env.json` file, APPCODE will be obtained in cz88 aliyun service

```json
{
  "APPCODE": "xxxx"
}
```

****1. Compile the program****

`go build .`


****2. Start the first node****
```cmd
.\StarryProxy.exe --p2p 9001 --proxy 9002
```
The result you will see:
```
Proxy server is ready
libp2p-peer addresses:
/ip4/127.0.0.1/tcp/9001/ipfs/QmcgYHUS7jnvBnBjxqHx4Wh7HZYfcZLA3oKK6oLgxGT2TZ
```

****3. Start the normal node****
```cmd
.\StarryProxy.exe -p2p 8001 -proxy 8002 -snid /ip4/127.0.0.1/tcp/9001/ipfs/QmcgYHUS7jnvBnBjxqHx4Wh7HZYfcZLA3oKK6oLgxGT2TZ
```
The result you will see:

```
Proxy server is ready
libp2p-peer addresses:
/ip4/127.0.0.1/tcp/8001/ipfs/QmNVs2BR1XxwEEEvUXQu5PTCtfTJsdZGaLdfWyAGQhwZ6H
/ip4/127.0.0.1/tcp/8002
proxy listening on  127.0.0.1:8002
```
