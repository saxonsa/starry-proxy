package config

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
)

type Position struct {
	Province string `json:"Province"`
	City string `json:"City"`
}

type Config struct {
	Version string `json:"Version"`

	SuperNode Node `json:"SuperNode"`

	Proxy Proxy `json:"Proxy"`

	P2P P2P `json:"P2P"`

	Position Position `json:"Position"`
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
	flag.IntVar(&cfg.Proxy.Port, "proxy", cfg.Proxy.Port, "proxy port")
	flag.IntVar(&cfg.P2P.Port, "p2p", cfg.P2P.Port, "p2p port")
	flag.StringVar(&cfg.Position.Province, "province", cfg.Position.Province, "province")
	flag.StringVar(&cfg.Position.City, "city", cfg.Position.City, "city")
	flag.Parse()

	return &cfg, nil
}
