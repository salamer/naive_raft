package main

import (
	"fmt"

	raft "github.com/salamer/naive_raft"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	name = kingpin.Flag("name", "input the server name").Short('n').Required().String()
	id   = kingpin.Flag("Id", "input the server id").Short('i').Required().Int()
	host = kingpin.Flag("host", "input the server host").Short('h').Required().String()
	port = kingpin.Flag("port", "input the server port").Short('p').Required().Int()
)

func main() {
	kingpin.Parse()
	node := raft.NewNode(*name, *id, "/Users/chenyangyang/go/src/github.com/salamer/naive_raft/conf.json")
	fmt.Printf("I'm listening %+v:%+v\n", *host, *port)
	node.Run(*host, *port)
}
