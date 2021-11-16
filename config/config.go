package config

import (
	"StarryProxy/ip"
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

	Position ip.Position `json:"Position"`

	IP string `json:"IP"`

	Name string `json:"Name"`

	Demo bool `json:"Demo"`

	Rate float64 `json:"Rate"`
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
	bytes, err := ioutil.ReadFile("./config/config.json")
	if err != nil {
		log.Fatalf("Fail to read config.json")
		return nil, err
	}
	err = json.Unmarshal(bytes, &cfg)

	// get peer position
	cfg.IP, cfg.Position, err = ip.GetLocalPosition()
	if err != nil {
		log.Printf("Fail to get local position from cz88 api: %s", err)
	}

	flag.StringVar(&cfg.SuperNode.Id, "snid", "", "supernode id used to enter the p2p net")
	flag.IntVar(&cfg.Proxy.Port, "proxy", cfg.Proxy.Port, "proxy port")
	flag.IntVar(&cfg.P2P.Port, "p2p", cfg.P2P.Port, "p2p port")
	flag.StringVar(&cfg.Position.Province, "province", cfg.Position.Province, "province")
	flag.StringVar(&cfg.Position.City, "city", cfg.Position.City, "city")
	flag.StringVar(&cfg.Name, "", cfg.Name, "p2p node name")
	flag.BoolVar(&cfg.Demo, "demo", cfg.Demo, "")
	flag.Float64Var(&cfg.Rate, "rate", cfg.Rate, "init rate for testing")

	flag.Parse()

	return &cfg, nil
}
