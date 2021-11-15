package protocol

const (
	HTTPProxyProtocol = "/starry-proxy/http/0.0.1" // http or https stream

	NewNodeProtocol = "/starry-proxy/new-node-entry/0.0.1"

	CommonManageProtocol = "/starry-proxy/common-manage-protocol/0.0.1"
)

const (
	EXIT = iota

	PeerList

	SNList

	UpdatePeerList

	UpdateSNList

	AllClusterList

	AssignSelfAsSupernode

	ExistedSupernodeInSelfCluster

	AliveTest

	AliveTestAck

	NodeLeave
)
