package parameters

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
)

type Parameter struct {
	Delay float64 `json:"Delay"`
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
	flag.Parse()

	return &params, nil
}
