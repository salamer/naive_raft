package naive_raft

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
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
	INTERVAL = 5 //heartbeat INTERVAL
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
	leaderId        int       //record the leader now
	heartbeatSignal chan bool //the heartbeat channel
	finishState     chan bool
	siblingNodes    []NodeConf
	electionResCnt  int //count for the election request result
	votedCnt        int //count for the server which has vote for it
	observersLock   sync.RWMutex
	actionLock      sync.RWMutex //in some action function
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
		finishState:     make(chan bool),
		siblingNodes:    loadNodesConf(conf),
	}
}

func (node *Node) getState() int {
	return node.state
}

func (node *Node) termIncrement() {
	node.actionLock.Lock()
	node.currentTerm += 1
	node.actionLock.Unlock()
}

func (node *Node) gotHeartbeat() {
	node.heartbeatSignal <- true
}

func (node *Node) setState(state int) error {
	if state >= 0 && state < 3 {
		node.actionLock.Lock()
		node.state = state
		node.actionLock.Unlock()
		return nil
	}
	return StateErr
}

func (node *Node) setLeader(leaderId int) {
	node.actionLock.Lock()
	node.leaderId = leaderId
	node.actionLock.Unlock()
}

func (node *Node) loop() {
	for {
		fmt.Printf("%+v : term is %+v,leader is %+v\n", node.name, node.currentTerm, node.leaderId)
		switch node.getState() {
		case FOLLOWER:
			fmt.Println(node.name + ": I'm follower now")
			node.follower_loop()
		case CANDIDATE:
			fmt.Println(node.name + ": I'm candidate now")
			node.candidate_loop()
		case LEADER:
			fmt.Println(node.name + ": I'm leader now")
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
			node.setState(CANDIDATE)
			flag = false
		case <-node.heartbeatSignal:
			fmt.Println("got a heartbeat")
		case <-node.finishState:
			flag = false
		}
	}
}

func (node *Node) candidate_loop() {
	flag := true
	for flag {
		//initialize node condition
		node.actionLock.Lock()
		node.electionResCnt = 1
		node.votedCnt = 1 // vote for itself
		node.votedFor = node.id
		node.actionLock.Unlock()
		node.setLeader(node.id)
		node.termIncrement() //term++
		_ = node.Elect()
		fmt.Printf("candidate %+v: my term is %+v\n", node.name, node.currentTerm)
		select {
		case <-time.After(time.Second * time.Duration(INTERVAL+rand.Intn(INTERVAL))):
			fmt.Println("candidate timeout")
		case <-node.finishState:
			flag = false
		}
	}
}

func (node *Node) leader_loop() {
	flag := true
	for flag {
		select {
		case <-time.After(time.Second * time.Duration(rand.Intn(INTERVAL))):
			_ = node.AppendEntry()
		case <-node.finishState:
			flag = false
		}
	}
}

//RPC function
func (node *Node) getEntris(start int, end int) []*pb.LogEntris {
	ResEntris := make([]*pb.LogEntris, start-end)
	for i := start; i < end; i++ {
		ResEntris[i].Index = int32(node.log[i].idx)
		ResEntris[i].Term = int32(node.log[i].term)
		ResEntris[i].Data = node.log[i].data
	}
	return ResEntris
}

func (node *Node) AppendEntry() error {
	if node.getState() == LEADER {
		for _, sibling := range node.siblingNodes {
			if sibling.ID != node.id {
				go func(host string, port int) {
					conn, e := grpc.Dial(host+":"+strconv.Itoa(port), grpc.WithInsecure())
					if e != nil {
						fmt.Printf("did not connect: %v", e)
					}

					c := pb.NewAppendEntriesClient(conn)
					node.observersLock.RLock()
					_, err := c.AppendEntriesRPC(context.Background(), &pb.AppendEntriesReq{
						Term:          int32(node.currentTerm),
						LeaderId:      int32(node.id),
						PrevLogIndex:  int32(node.commitIndex),
						PrevTermIndex: int32(node.commitIndex),
						LogEntris:     node.getEntris(0, len(node.log)),
						LeaderCommit:  int32(node.commitIndex),
					})
					if err != nil {
						fmt.Printf("append entries error: %v\n", err)
					}
					node.observersLock.RUnlock()
					defer conn.Close()
				}(sibling.Host, sibling.Port)
			}
		}
		return nil
	}
	return NotLeaderErr
}

func (node *Node) AppendEntriesRPC(ctx context.Context, in *pb.AppendEntriesReq) (*pb.AppendEntriesResp, error) {
	if node.getState() != FOLLOWER {
		node.setState(FOLLOWER)
		node.finishState <- true
		node.setLeader(int(in.LeaderId))
		node.currentTerm = int(in.Term)
	} else {
		node.heartbeatSignal <- true
	}
	return &pb.AppendEntriesResp{Term: 1, Success: true}, nil
}

func (node *Node) Elect() error {
	if node.getState() == CANDIDATE {
		for _, sibling := range node.siblingNodes {
			if sibling.ID != node.id {
				go func(host string, port int, sibling NodeConf) {
					conn, e := grpc.Dial(host+":"+strconv.Itoa(port), grpc.WithInsecure())
					if e != nil {
						fmt.Printf("did not connect: %v", e)
					}

					c := pb.NewElectionClient(conn)
					node.observersLock.RLock()
					r, err := c.ElectionRPC(context.Background(), &pb.ElectionReq{
						Term:         int32(node.currentTerm),
						CandidateId:  int32(node.id),
						LastLogIndex: int32(node.commitIndex),
						LastLogTerm:  int32(node.currentTerm),
					})
					node.observersLock.RUnlock()
					//means has already get res from server i
					node.observersLock.Lock()
					node.electionResCnt += 1
					fmt.Printf("vote:%+v,res:%+v\n", node.votedCnt, node.electionResCnt)
					if err != nil {
						fmt.Printf("election error: %v\n", err)
					} else {
						if r.VotedGranted {
							fmt.Printf("%+v got vote from  %+v\n", node.id, sibling.ID)
							node.votedCnt += 1
						}
					}
					node.observersLock.Unlock()
					if node.votedCnt >= len(node.siblingNodes)/2 {
						node.setState(LEADER)
						node.finishState <- true
					} else {
						// eletion fail,return to de follower state
						if node.electionResCnt >= len(node.siblingNodes)/2 {
							node.setState(FOLLOWER)
							node.finishState <- true
						}
					}

					defer conn.Close()
				}(sibling.Host, sibling.Port, sibling)
			}
		}
		return nil
	}
	return NotLeaderErr
}

func (node *Node) ElectionRPC(ctx context.Context, in *pb.ElectionReq) (*pb.ElectionResp, error) {
	node.gotHeartbeat()
	fmt.Printf("%+v got vote request by %+v\n", node.id, in.CandidateId)
	if node.currentTerm < int(in.Term) {
		node.currentTerm = int(in.Term)
		return &pb.ElectionResp{
			Term:         int32(node.currentTerm),
			VotedGranted: true,
		}, nil
	}
	return &pb.ElectionResp{
		Term:         int32(node.currentTerm),
		VotedGranted: false,
	}, nil
}
func (node *Node) Run(host string, port int) {
	//run node state loop
	go node.loop()

	lis, err := net.Listen("tcp", host+":"+strconv.Itoa(port))
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterAppendEntriesServer(s, node)
	pb.RegisterElectionServer(s, node)
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
