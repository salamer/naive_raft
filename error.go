package naive_raft

import "errors"

var (
	StateErr       = errors.New("wrone state")
	NotCandiateErr = errors.New("not candidate")
	NotLeaderErr   = errors.New("not leader")
	SetLogErr      = errors.New("can't set log")
)
