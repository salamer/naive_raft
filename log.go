package naive_raft

type Log struct {
	idx  int
	term int
	data string
}
