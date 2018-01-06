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
	name        string
	id          int
	currentTerm int
	votedFor    int
	log         []Log
	commitIndex int
	lastApplied int
	nextIndex   map[int]int
	matchIndex  map[int]int
	state       int
	leaderId    int //record the leader now

	heartbeatSignal chan bool //the heartbeat channel
	finishState     chan bool

	siblingNodes []NodeConf

	electionResCnt int //count for the election request result
	votedCnt       int //count for the server which has vote for it
	majoritySize   int

	observersLock sync.RWMutex
	actionLock    sync.RWMutex //in some action function
}

func NewNode(name string, id int, conf string) *Node {

	// initialize nextindex and matchindx to 0
	nextIndex := make(map[int]int)
	matchIndex := make(map[int]int)
	for i := 0; i < len(conf); i++ {
		nextIndex[i] = 0
		matchIndex[i] = 0
	}
	nodeconfs := loadNodesConf(conf)
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
		state:           LEADER,
		heartbeatSignal: make(chan bool),
		finishState:     make(chan bool),
		siblingNodes:    nodeconfs,
		majoritySize:    getMajoritySize(len(nodeconfs)),
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
		case <-time.After(time.Second * time.Duration(INTERVAL+rand.Intn(INTERVAL*2))):
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
		node.votedCnt = 1 // vote to itself
		node.votedFor = node.id
		node.actionLock.Unlock()
		node.setLeader(node.id)
		node.termIncrement() //term++
		_ = node.Canvass()
		select {
		case <-time.After(time.Second * time.Duration(rand.Intn(INTERVAL)*2)):
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
			//			_ = node.AppendEntry()
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
	if in.Term < int32(node.currentTerm) {
		return &pb.AppendEntriesResp{
			Term:    int32(node.currentTerm),
			Success: false,
		}, nil
	}
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

func (node *Node) Canvass() error {
	if node.getState() == CANDIDATE {
		for _, sibling := range node.siblingNodes {
			if sibling.ID != node.id {
				go func(host string, port int, sibling NodeConf) {
					conn, e := grpc.Dial(host+":"+strconv.Itoa(port), grpc.WithInsecure())
					if e != nil {
						fmt.Printf("did not connect: %v", e)
					}

					c := pb.NewCanvassClient(conn)
					r, err := c.CanvassRPC(context.Background(), &pb.CanvassReq{
						Term:         int32(node.currentTerm),
						CandidateId:  int32(node.id),
						LastLogIndex: int32(node.commitIndex),
						LastLogTerm:  int32(node.currentTerm),
					})
					//means has already get res from server i
					node.observersLock.Lock()
					node.electionResCnt += 1
					if err != nil {
						fmt.Printf("election error: %v\n", err)
					} else {
						if r.VotedGranted && r.Term == int32(node.currentTerm) {
							node.votedCnt += 1
						}
					}
					node.observersLock.Unlock()
					if node.votedCnt >= node.majoritySize {
						node.setState(LEADER)
						node.finishState <- true
					} else {
						// eletion fail,return to de follower state
						if node.electionResCnt >= node.majoritySize {
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

func (node *Node) CanvassRPC(ctx context.Context, in *pb.CanvassReq) (*pb.CanvassResp, error) {
	node.gotHeartbeat()
	if node.currentTerm < int(in.Term) {
		node.currentTerm = int(in.Term)
		return &pb.CanvassResp{
			Term:         int32(node.currentTerm),
			VotedGranted: true,
		}, nil
	}
	return &pb.CanvassResp{
		Term:         int32(node.currentTerm),
		VotedGranted: false,
	}, nil
}

func (node *Node) SetLogRPC(ctx context.Context, in *pb.LogReq) (*pb.LogResp, error) {
	if node.getState() == LEADER {
		if len(node.log) == 0 {
			node.lastApplied = 0
		}
		node.lastApplied += 1
		node.log = append(node.log, Log{
			idx:  node.lastApplied,
			term: node.currentTerm,
			data: in.Data,
		})

		fmt.Printf("%+v\n", node.log)
		return &pb.LogResp{
			Success: true,
		}, nil
	}
	return &pb.LogResp{
		Success: false,
	}, nil
}

func (node *Node) Run(host string, port int) {
	//run node state loop
	rand.Seed(time.Now().Unix())
	go node.loop()
	lis, err := net.Listen("tcp", host+":"+strconv.Itoa(port))
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterAppendEntriesServer(s, node)
	pb.RegisterCanvassServer(s, node)
	pb.RegisterSetLogServer(s, node)
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
