package naive_raft

type Log struct {
	idx  int
	term int
	data string
}

func (node *Node) setLog(data string) error {
	//TODO: SET log
	node.logReq <- true
	return nil
}
