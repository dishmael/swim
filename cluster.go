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
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Global Variables
var (
	logger        zerolog.Logger
	shutdownChan  chan bool
	nodeJoinChan  chan *rpc.Node
	nodeLeaveChan chan *rpc.Node
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

	shutdownChan = make(chan bool)
	nodeJoinChan = make(chan *rpc.Node)
	nodeLeaveChan = make(chan *rpc.Node)
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
	config  *Config
	members Collection
	self    *rpc.Node
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
	rpc := NewServer()
	rpc.StartListener(c.config.Address, int(c.config.RPCPort))

	// Start UDP Server
	udp := NewUDP()
	udp.StartListener(int(c.config.UDPPort))

	// Announce self to others
	if c.config.Seed != nil {
		c.sendJoin()
	} else {
		if c.config.Address == "" {
			bAddr, err := udp.GetBroadcastAddress()
			if err != nil {
				logger.Error().Msg(err.Error())
				return
			}
			c.config.Address = bAddr.String()
		}
		udp.SendBroadcast(c.config.Address, int(c.config.UDPPort), c.self)
	}

	// Entering the main cluster loop
	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-shutdownChan:
			rpc.Stop()
			udp.Stop()
			break

		case n := <-nodeJoinChan:
			logger.Debug().Msgf("joinChan -> %+v", n)
			if n.Address != c.self.Address {
				n.LastSeen = time.Now().UnixNano()
				c.members.AddNode(n)
			}

		case n := <-nodeLeaveChan:
			logger.Debug().Msgf("leaveChan -> %+v", n)
			if n.Address != c.self.Address {
				c.members.DeleteNode(n.Address)
			}

		case <-t.C:
			c.sendPing()
		}
	}
}

// Stop ...
func (c *cluster) Stop() {
	logger.Debug().Msg("Shutting down...")
	shutdownChan <- true
	waitGroup.Wait()
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
		Address:   c.config.Address,
	})

	return nil
}

// sendPing
func (c *cluster) sendPing() error {
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
	resp, err := client.Ping(context.Background(), &rpc.PingMessageInput{
		Timestamp: time.Now().UnixNano(),
		Source:    c.self,
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

	logger.Debug().Msgf("%+v", resp)
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
