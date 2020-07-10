package swim

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"time"

	rpc "github.com/dishmael/swim/rpc"
)

// Collection is an unordered grouping of Nodes
type Collection interface {
	AddNode(node *rpc.Node)
	DeleteNode(address string)
	GetKeys() []string
	GetNodes() []*rpc.Node
	GetNodeByAddress(address string) (*rpc.Node, bool)
	GetRandomNode() (*rpc.Node, error)
	GetRandomNodes(size int) []*rpc.Node
	GetSampleSize(zscore float64, moe float64) int
	GetSortedNodes(o SortOrder) []*rpc.Node
	Len() int
}
type collection struct {
	members map[string]*rpc.Node
}

// NewCollection initializes our collection
func NewCollection() Collection {
	return &collection{
		members: make(map[string]*rpc.Node),
	}
}

// AddNode adds a Node to the collection using address as the key
func (c *collection) AddNode(n *rpc.Node) {
	c.members[n.Address] = n
}

// DeleteNode removes a specific Node from the collection using the address (key)
func (c *collection) DeleteNode(address string) {
	delete(c.members, address)
}

// GetKeys ...
func (c *collection) GetKeys() []string {
	keys := make([]string, len(c.members))
	for k := range c.members {
		keys = append(keys, k)
	}

	return keys
}

// GetNodes returns an array (slice) of Nodes
func (c *collection) GetNodes() []*rpc.Node {
	nodes := make([]*rpc.Node, 0)
	for _, n := range c.members {
		nodes = append(nodes, n)
	}

	return nodes
}

// GetNodeByAddress returns a single Node based on the address
func (c *collection) GetNodeByAddress(address string) (*rpc.Node, bool) {
	n, ok := c.members[address]
	return n, ok
}

// GetRandomNode returns a single random Node from the collection
func (c *collection) GetRandomNode() (*rpc.Node, error) {
	if c.Len() == 0 {
		return nil, errors.New("Emptry collection")
	}

	return c.GetRandomNodes(1)[0], nil
}

// GetRandomNodes returns a slice of random Nodes from the collection
func (c *collection) GetRandomNodes(size int) []*rpc.Node {
	nodes := c.GetNodes()

	if c.Len() == 0 || size == 0 {
		return make([]*rpc.Node, 0)
	}

	// Randomize the slize
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })

	if c.Len() >= size {
		return nodes
	}

	return nodes[:size-1]
}

// GetSampleSize returns the ideal sample size from the collection (finite population)
// given a known Z-Score (e.g. 1.96 == 97.5%) and margin of error (moe).
func (c *collection) GetSampleSize(zscore float64, moe float64) int {
	uP := math.Round(math.Pow(zscore, 2) * 0.5 * 0.5 / math.Pow(moe, 2))
	fP := 1 + (math.Round((float64(len(c.members)) * uP) / (uP + float64(len(c.members)) - 1)))
	return int(fP)
}

// GetSortedNodes returns an ordered slice of Nodes
func (c *collection) GetSortedNodes(o SortOrder) []*rpc.Node {
	nodes := c.GetNodes()

	switch o {
	case ASC:
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].LastSeen < nodes[j].LastSeen
		})
	case DESC:
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].LastSeen > nodes[j].LastSeen
		})
	}

	return nodes
}

// Len returns the size of the Node collection
func (c *collection) Len() int {
	return len(c.members)
}
