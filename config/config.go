package config

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
)

type Config struct {
	Version string `json:"Version"`

	SuperNode Node `json:"SuperNode"`

	Proxy Proxy `json:"Proxy"`

	P2P P2P `json:"P2P"`
}

type Node struct {
	Id string `json:"Id"`
}

type Proxy struct {
	Port int `json:"Port"`
}

type P2P struct {
	Port int `json:"Port"`
}

func InitConfig() (*Config, error) {
	cfg := Config{}
	bytes, err := ioutil.ReadFile("./config.json")
	if err != nil {
		log.Fatalf("Fail to read config.json")
		return nil, err
	}
	err = json.Unmarshal(bytes, &cfg)

	flag.StringVar(&cfg.SuperNode.Id, "snid", "", "supernode id used to enter the p2p net")
	flag.IntVar(&cfg.Proxy.Port, "proxy", 5019, "proxy port")
	flag.IntVar(&cfg.P2P.Port, "p2p", 10001, "p2p port")
	flag.Parse()

	return &cfg, nil
}
