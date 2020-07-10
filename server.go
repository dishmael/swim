package swim

import (
	"context"
	"fmt"
	"net"
	"time"

	rpc "github.com/dishmael/swim/rpc"
	"google.golang.org/grpc"
)

// Server ...
type Server interface {
	StartListener(address string, port int) error
	Stop()
	Join(ctx context.Context, in *rpc.JoinMessageInput) (*rpc.JoinMessageOutput, error)
	Leave(ctx context.Context, in *rpc.LeaveMessageInput) (*rpc.LeaveMessageOutput, error)
	Ping(ctx context.Context, in *rpc.PingMessageInput) (*rpc.PingMessageOutput, error)
	PingRequest(ctx context.Context, in *rpc.PingRequestMessageInput) (*rpc.PingMessageOutput, error)
}

type server struct {
	rpc.Node

	server *grpc.Server
}

// NewServer ...
func NewServer() Server {
	return &server{}
}

// StartListener ...
func (s *server) StartListener(address string, port int) error {
	logger.Debug().Str("address", address).Int("port", port).Msg("Starting RPC server")

	// Configure the RPC server
	s.server = grpc.NewServer()
	rpc.RegisterSwimServer(s.server, s)
	glisten, err := net.Listen("tcp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return err
	}

	// Start listening for RCP requests
	waitGroup.Add(1)
	go s.server.Serve(glisten)
	logger.Debug().Msg("RPC server listening")
	return nil
}

func (s *server) Stop() {
	s.server.Stop()
	logger.Debug().Msg("RPC server stopped")
	waitGroup.Done()
}

//
// SwimServer Functions (called by remote nodes)
//

// Join ...
func (s *server) Join(ctx context.Context, in *rpc.JoinMessageInput) (*rpc.JoinMessageOutput, error) {
	nodeJoinChan <- in.GetNode()

	return &rpc.JoinMessageOutput{
		Timestamp: time.Now().UnixNano(),
		Success:   true,
	}, nil
}

// Leave removes a Node from the collection
func (s *server) Leave(ctx context.Context, in *rpc.LeaveMessageInput) (*rpc.LeaveMessageOutput, error) {
	nodeLeaveChan <- in.GetNode()

	return &rpc.LeaveMessageOutput{
		Timestamp: time.Now().UnixNano(),
		Success:   true,
	}, nil
}

// Ping ...
func (s *server) Ping(ctx context.Context, in *rpc.PingMessageInput) (*rpc.PingMessageOutput, error) {
	// Add the source
	nodePingChan <- in.Source

	// Add joins
	joins := in.GetJoins()
	for _, j := range joins {
		nodeJoinChan <- j
	}

	// Remove departures
	departures := in.GetLeaves()
	for _, d := range departures {
		nodeLeaveChan <- d
	}

	return &rpc.PingMessageOutput{
		Timestamp: time.Now().UnixNano(),
		Success:   true,
	}, nil
}

// PingRequest sends a Ping request on behalf of another Node
func (s *server) PingRequest(ctx context.Context, in *rpc.PingRequestMessageInput) (*rpc.PingMessageOutput, error) {
	return &rpc.PingMessageOutput{
		Timestamp: time.Now().UnixNano(),
		Success:   true,
	}, nil
}
