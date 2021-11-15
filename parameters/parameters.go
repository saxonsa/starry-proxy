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
	Delay float64 `json:"Delay"`
	Period Period `json:"Period"`
}

func InitParameter() (*Parameter, error) {
	params := Parameter{}
	bytes, err := ioutil.ReadFile("./parameters/parameters.json")
	if err != nil {
		log.Fatalf("Fail to read config.json")
		return nil, err
	}
	err = json.Unmarshal(bytes, &params)

	flag.Float64Var(&params.Delay, "delay", params.Delay, "delay for transmit http request")
	flag.IntVar(&params.Period.SnList, "sn_period", params.Period.SnList, "period to do heart beat test for snList")
	flag.IntVar(&params.Period.PeerList, "peer_period", params.Period.PeerList, "period to do heart beat test for PeerList")
	flag.Parse()

	return &params, nil
}
