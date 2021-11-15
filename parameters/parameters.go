package parameters

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
)

type HeartBeat struct {
	SnList int `json:"SnList"`
	PeerList int `json:"PeerList"`
}

type Parameter struct {
	HeartBeat HeartBeat `json:"HeartBeat"`
	BandWidth float64 `json:"BandWidth"`
	ClusterUpdate float64 `json:"ClusterUpdate"`
}

func InitParameter() (*Parameter, error) {
	params := Parameter{}
	bytes, err := ioutil.ReadFile("./parameters/parameters.json")
	if err != nil {
		log.Fatalf("Fail to read config.json")
		return nil, err
	}
	err = json.Unmarshal(bytes, &params)

	flag.Float64Var(&params.BandWidth, "bandwidth", params.BandWidth, "init a bandwidth for testing")
	flag.IntVar(&params.HeartBeat.SnList, "sn_period", params.HeartBeat.SnList, "period to do heart beat test for snList")
	flag.IntVar(&params.HeartBeat.PeerList, "peer_period", params.HeartBeat.PeerList, "period to do heart beat test for PeerList")
	flag.Float64Var(&params.ClusterUpdate, "ClusterUpdate", params.ClusterUpdate, "cluster update period")
	flag.Parse()

	return &params, nil
}
