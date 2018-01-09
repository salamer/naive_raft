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

//return the last log
func (node *Node) getFirst() (Log, error) {
	if len(node.log) > 0 {
		return node.log[len(node.log)-1], nil
	} else {
		return Log{}, NoLogErr
	}
}

func (node *Node) getLogs() ([]Log, error) {
	if len(node.log) > 0 {
		return node.log, nil
	} else {
		return nil, NoLogErr
	}
}
