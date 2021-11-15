package protocol

const (
	HTTPProxyProtocol = "/starry-proxy/http/0.0.1" // http or https stream

	NewNodeProtocol = "/starry-proxy/new-node-entry/0.0.1"

	NodeAliveTestProtocol = "/starry-proxy/node-alive-test-protocol/0.0.1"

	UpdateClusterProtocol = "/starry-proxy/update-cluster-protocol/0.0.1"
)

const (
	EXIT = iota

	PeerList

	SNList

	AllClusterList

	AssignSelfAsSupernode

	ExistedSupernodeInSelfCluster

	AliveTest

	AliveTestAck
)
