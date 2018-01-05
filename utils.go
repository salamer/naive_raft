package naive_raft

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

func loadNodesConf(confPath string) []NodeConf {
	file, e := ioutil.ReadFile(confPath)
	if e != nil {
		fmt.Printf("File error: %v\n", e)
		os.Exit(1)
	}
	var nodeconfs []NodeConf
	json.Unmarshal(file, &nodeconfs)
	return nodeconfs
}

func getMajoritySize(size int) int {
	return size / 2
}
