package protocol

const (
	HTTPProxyProtocol = "/starry-proxy/http/0.0.1" // http or https stream

	NewNodeProtocol = "/starry-proxy/new-node-entry/0.0.1"
)

const (
	EXIT = 100

	PeerList = 101

	AssignSelfAsSupernode = 102

	ExistedSupernodeInSelfCluster = 103
)
