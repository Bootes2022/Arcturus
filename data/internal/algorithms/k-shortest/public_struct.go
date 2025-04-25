package k_shortest

// the numbers of nodes in a network start from 0, e.g., 10 nodes are 0-9.
type Network struct {
	Nodes []Node  `json:"nodes"`
	Links [][]int `json:"links"` // latency of every link, -1 means that there is no link between two nodes
}

type Node struct {
}

type Result struct {
	Ip1   string
	Ip2   string
	Value float64
}

type Flow struct {
	Source      int `json:"source"`
	Destination int `json:"destination"`
}

type Path struct {
	Nodes   []int `json:"nodes"`   // nodes on a path
	Latency int   `json:"latency"` // total latency of a path
}

type PathWithIP struct {
	IPList  []string
	Latency int
	Weight  int
}

type RoutingResult struct {
	Net   Network `json:"net"`
	Flows []Flow  `json:"flows"`
}

type Edge struct {
	From, To, Capacity int
}

type Graph struct {
	Capacity [][]int
	Flow     [][]int
	Nodes    int
}
