package parameters

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
)

type Period struct {
	SnList int `json:"SnList"`
	PeerList int `json:"PeerList"`
}

type Parameter struct {
	Period Period `json:"Period"`
	BandWidth float64 `json:"BandWidth"`
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
	flag.IntVar(&params.Period.SnList, "sn_period", params.Period.SnList, "period to do heart beat test for snList")
	flag.IntVar(&params.Period.PeerList, "peer_period", params.Period.PeerList, "period to do heart beat test for PeerList")
	flag.Parse()

	return &params, nil
}
