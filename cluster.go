package swim

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	rpc "github.com/dishmael/swim/rpc"
	"github.com/dishmael/ztable"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Global Variables
var (
	logger        zerolog.Logger
	nodeJoinChan  chan *rpc.Node
	nodeLeaveChan chan *rpc.Node
	nodePingChan  chan *rpc.Node
	waitGroup     sync.WaitGroup
)

// Initialization
func init() {
	// Initialize Logger
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("| %-5s |", i))
	}

	//zerolog.SetGlobalLevel(zerolog.InfoLevel)
	logger = zerolog.New(output).With().Timestamp().Logger()

	nodeJoinChan = make(chan *rpc.Node)
	nodeLeaveChan = make(chan *rpc.Node)
	nodePingChan = make(chan *rpc.Node)
}

// Config ...
type Config struct {
	Address          string
	BroadcastAddress string
	Seed             *rpc.Node
	RPCPort          uint32
	UDPPort          uint32
}

// DefaultConfig ...
func DefaultConfig() *Config {
	return &Config{
		Address: "127.0.0.1",
		RPCPort: 50051,
		UDPPort: 50052,
	}
}

// Cluster ...
type Cluster interface {
	GetMembers() []*rpc.Node
	Start()
	Stop()
}

type cluster struct {
	sync.Mutex

	config    *Config
	joins     []*rpc.Node
	leaves    []*rpc.Node
	members   Collection
	rpcServer Server
	self      *rpc.Node
	udpServer UDP
}

// NewCluster ...
func NewCluster(config *Config) Cluster {
	return &cluster{
		config:  config,
		members: NewCollection(),
		self: &rpc.Node{
			Address:  config.Address,
			Port:     config.RPCPort,
			LastSeen: time.Now().UnixNano(),
		},
	}
}

// GetMembers ...
func (c *cluster) GetMembers() []*rpc.Node {
	return c.members.GetNodes()
}

// Start ...
func (c *cluster) Start() {
	logger.Debug().Msg("Starting SWIM Cluster")

	// Register oour signal handler for clean shutdowns
	go c.handleSigTerm()

	// Start RPC Server
	c.rpcServer = NewServer()
	c.rpcServer.StartListener(c.config.Address, int(c.config.RPCPort))

	// Start UDP Server
	c.udpServer = NewUDP()
	c.udpServer.StartListener(int(c.config.UDPPort))

	// Announce self to others
	if c.config.Seed != nil {
		c.sendJoin()
	} else {
		if len(c.config.BroadcastAddress) == 0 {
			bAddr, err := c.udpServer.GetBroadcastAddress()
			if err != nil {
				logger.Error().Msg(err.Error())
				return
			}

			c.config.BroadcastAddress = bAddr.String()
		}

		logger.Debug().Str("address", c.config.BroadcastAddress).Msg("Sending broadcast announcement")
		c.udpServer.SendBroadcast(c.config.BroadcastAddress, int(c.config.UDPPort), c.self)
	}

	// Entering the main cluster loop
	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case n := <-nodeJoinChan:
			if n.Address != c.self.Address {
				logger.Debug().Msgf("joinChan -> %+v", n)
				c.Lock()
				n.LastSeen = time.Now().UnixNano()
				c.members.AddNode(n)
				c.joins = append(c.joins, n)
				c.Unlock()
			}

		case n := <-nodeLeaveChan:
			if n.Address != c.self.Address {
				logger.Debug().Msgf("leaveChan -> %+v", n)
				c.Lock()
				c.members.DeleteNode(n.Address)
				c.leaves = append(c.leaves, n)
				c.Unlock()
			}

		case n := <-nodePingChan:
			if n.Address != c.self.Address {
				c.Lock()
				n.LastSeen = time.Now().UnixNano()
				c.members.AddNode(n)
				c.Unlock()
			}
		case <-t.C:
			c.sendPing()
		}
	}
}

// Stop ...
func (c *cluster) Stop() {
	logger.Debug().Msg("Shutting down...")
	c.rpcServer.Stop()
	c.udpServer.Stop()
	waitGroup.Wait()
}

// getJoins ...
func (c *cluster) getJoins() []*rpc.Node {
	c.Lock()
	joins := c.joins
	c.joins = make([]*rpc.Node, 0)
	c.Unlock()
	return joins
}

// getLeaves ...
func (c *cluster) getLeaves() []*rpc.Node {
	c.Lock()
	leaves := c.leaves
	c.leaves = make([]*rpc.Node, 0)
	c.Unlock()
	return leaves
}

// sendJoin
func (c *cluster) sendJoin() error {
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", c.config.Seed.Address, c.config.Seed.Port), grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()

	client := rpc.NewSwimClient(conn)
	_, err = client.Join(context.Background(), &rpc.JoinMessageInput{
		Timestamp: time.Now().UnixNano(),
	})

	if err != nil {
		switch status.Code(err) {
		case codes.Unavailable:
			logger.Error().Str("action", "join").Msg("Seed Node is Unavailable")
			return err

		default:
			logger.Error().Str("action", "join").Msg(err.Error())
			return errors.New(err.Error())
		}
	}

	return nil
}

// sendLeave
func (c *cluster) sendLeave() error {
	n, err := c.members.GetRandomNode()
	if err != nil {
		return err
	}

	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", n.Address, n.Port), grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()

	client := rpc.NewSwimClient(conn)
	client.Leave(context.Background(), &rpc.LeaveMessageInput{
		Timestamp: time.Now().UnixNano(),
		Node:      c.self,
	})

	return nil
}

// sendPing
func (c *cluster) sendPing() error {
	ztable := ztable.NewTable()
	size := c.members.GetSampleSize(ztable.GetScore(0.80), 0.05)
	nodes := c.members.GetRandomNodes(size)

	joins := c.getJoins()
	leaves := c.getLeaves()

	for _, n := range nodes {
		conn, err := grpc.Dial(fmt.Sprintf("%s:%d", n.Address, n.Port), grpc.WithInsecure())
		if err != nil {
			return err
		}
		defer conn.Close()

		client := rpc.NewSwimClient(conn)
		_, err = client.Ping(context.Background(), &rpc.PingMessageInput{
			Timestamp: time.Now().UnixNano(),
			Source:    c.self,
			Joins:     joins,
			Leaves:    leaves,
		})

		if err != nil {
			switch status.Code(err) {
			case codes.Unavailable:
				logger.Error().Str("action", "ping").Msg("Node is Unavailable")
				//TODO: Try PingRequest
				nodeLeaveChan <- n
				return err

			default:
				logger.Error().Str("action", "join").Msg(err.Error())
				return errors.New(err.Error())
			}
		}
	}

	return nil
}

// sendPingRequest
func (c *cluster) sendPingRequest(n *rpc.Node) error {
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", n.Address, n.Port), grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()

	client := rpc.NewSwimClient(conn)
	client.PingRequest(context.Background(), &rpc.PingRequestMessageInput{
		Timestamp:  time.Now().UnixNano(),
		Source:     c.self,
		RemoteNode: n,
	})

	return nil
}

// handleSigTerm ensures we send a Leave message before shutting down
func (c *cluster) handleSigTerm() {
	// Enable the capture of Ctrl-C, to cleanly close the application
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, os.Kill)

	for {
		// Wait for signal
		select {
		case <-ch:
			logger.Debug().Msg("Signal received")
			c.Stop()
			os.Exit(-1)
		}
	}
}
