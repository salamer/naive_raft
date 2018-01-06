package naive_raft

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

func max(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func min(a int, b int) int {
	if a > b {
		return b
	} else {
		return a
	}
}

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
