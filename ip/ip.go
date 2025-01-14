package ip

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

type Position struct {
	Province string `json:"Province"`
	City string `json:"City"`
}

type APP struct {
	APPCode string `json:"APPCODE"`
}

type res struct {
	Code   int      `json:"code"`
	Data   Position `json:"data"`
}

func GetPublicIP() (string, error){
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	return string(body), nil
}

func GetPositionFromIP(ip string) (Position, error) {
	APP := APP{}
	bytes, err := ioutil.ReadFile("./env.json")
	if err != nil {
		log.Fatalf("Fail to open env.json file: %s\n", err)
	}
	err = json.Unmarshal(bytes, &APP)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://cz88.rtbasia.com/search?ip=" + ip, nil)
	req.Header.Add("Authorization", "APPCODE " + APP.APPCode)
	resp, _ := client.Do(req)
	body, _ := ioutil.ReadAll(resp.Body)

	res := res{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		log.Fatalln("fail to get position through cz88 api...")
	}
	if res.Code != 200 {
		log.Fatalf("cz88 return %d as error code: \n", res.Code)
	}

	return res.Data, nil
}

func GetLocalPosition() (string, Position, error) {
	PublicIP, err := GetPublicIP()
	if err != nil {
		log.Fatalln("Fail to get public ip")
	}
	Position, err := GetPositionFromIP(PublicIP)
	if err != nil {
		log.Fatalln("Fail to get position from ip")
	}
	return PublicIP, Position, nil
}
