package naive_raft

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"time"

	pb "github.com/salamer/naive_raft/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	FOLLOWER = iota
	CANDIDATE
	LEADER
)

const (
	INTERVAL = 4 //heartbeat INTERVAL
)

type NodeConf struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type Node struct {
	name            string
	id              int
	currentTerm     int
	votedFor        int
	log             []Log
	commitIndex     int
	lastApplied     int
	nextIndex       []int
	matchIndex      []int
	state           int
	heartbeatSignal chan bool //the heartbeat channel
	siblingNodes    []NodeConf
}

func NewNode(name string, id int, conf string) *Node {

	// initialize nextindex and matchindx to 0
	nextIndex := make([]int, len(conf))
	matchIndex := make([]int, len(conf))
	for i := 0; i < len(conf); i++ {
		nextIndex[i] = 0
		matchIndex[i] = 0
	}
	return &Node{
		name:            name,
		id:              id,
		currentTerm:     0, //initialize to 0
		votedFor:        0,
		log:             []Log{},
		commitIndex:     0,
		lastApplied:     0,
		nextIndex:       nextIndex,
		matchIndex:      matchIndex,
		state:           FOLLOWER,
		heartbeatSignal: make(chan bool),
		siblingNodes:    loadNodesConf(conf),
	}
}

func (node *Node) getState() int {
	return node.state
}

func (node *Node) setState(state int) error {
	if state >= 0 && state < 3 {
		node.state = state
		return nil
	}
	return StateErr
}

func (node *Node) loop() {
	for {
		switch node.getState() {
		case FOLLOWER:
			fmt.Println("I'm follower now")
			node.follower_loop()
		case CANDIDATE:
			fmt.Println("I'm candidate now")
			node.candidate_loop()
		case LEADER:
			fmt.Println("I'm leader now")
			node.leader_loop()
		}
	}
}

func (node *Node) follower_loop() {
	flag := true
	for flag {
		select {
		case <-time.After(time.Second * time.Duration(INTERVAL+rand.Intn(INTERVAL))):
			fmt.Println("timeout!")
			//			node.setState(CANDIDATE)
		case <-node.heartbeatSignal:
			fmt.Println("got a heartbeat")
		}
	}
}

func (node *Node) candidate_loop() {
	fmt.Println("I'm candidate now")
	flag := true
	for flag {
		select {
		case <-time.After(time.Second * time.Duration(INTERVAL+rand.Intn(INTERVAL))):
			node.setState(CANDIDATE)
		case <-node.heartbeatSignal:
			fmt.Println("got a heartbeat")
		}
	}
}

func (node *Node) leader_loop() {
	flag := true
	for flag {
		select {
		case <-time.After(time.Second * time.Duration(INTERVAL+rand.Intn(INTERVAL))):
			node.setState(CANDIDATE)
		case <-node.heartbeatSignal:
			fmt.Println("got a heartbeat")
		}
	}
}

//RPC function

func (node *Node) AppendEntriesRPC(ctx context.Context, in *pb.AppendEntriesReq) (*pb.AppendEntriesResp, error) {
	return &pb.AppendEntriesResp{Term: 1, Success: true}, nil
}

func (node *Node) Run(port int) {
	node.loop()
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterAppendEntriesServer(s, node)
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
