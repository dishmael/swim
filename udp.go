package swim

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"

	rpc "github.com/dishmael/swim/rpc"
)

// UDP ...
type UDP interface {
	GetBroadcastAddress() (net.IP, error)
	SendBroadcast(address string, port int, node *rpc.Node) (int, error)
	StartListener(port int) error
	Stop()
}
type udp struct {
	isRunning bool
	server    *net.UDPConn
}

// NewUDP ...
func NewUDP() UDP {
	return &udp{
		isRunning: false,
	}
}

// GetBroadcastAddress will go through the motion of setting up a
// connection, which forces the system to provide the default
// interface's IP address. We use that address to then determine
// the appropriate broadcast address. There might be a better way,
// but this seems to work fairly well so far.
func (u *udp) GetBroadcastAddress() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ipnet := &net.IPNet{
		IP:   localAddr.IP,
		Mask: localAddr.IP.DefaultMask(),
	}

	ip := make(net.IP, len(ipnet.IP.To4()))
	binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(
		ipnet.IP.To4())|^binary.BigEndian.Uint32(net.IP(ipnet.Mask).To4()))

	return ip, nil
}

// SendBroadcast ...
func (u *udp) SendBroadcast(address string, port int, node *rpc.Node) (int, error) {
	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return 0, err
	}

	c, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	data, err := json.Marshal(node)
	if err != nil {
		return 0, err
	}

	return c.Write(data)
}

// StartListener ...
func (u *udp) StartListener(port int) error {
	logger.Debug().Int("port", port).Msg("Starting UDP server")

	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	u.server, err = net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return err
	}

	// Listen for connections
	u.isRunning = true
	waitGroup.Add(1)
	go func() {
		buffer := make([]byte, 1024)

		for u.isRunning == true {
			// Listen for incoming UDP requests
			bytesRead, _, err := u.server.ReadFromUDP(buffer)
			if err != nil {
				if u.isRunning == true {
					logger.Error().Str("action", "handle UDP message").Msg(err.Error())
					continue
				} else {
					break
				}

			}

			// Marshal a Node into a JSON byte array
			n := &rpc.Node{}
			err = json.Unmarshal(buffer[0:bytesRead], n)
			if err != nil {
				logger.Error().Str("action", "marshal UDP message").Msg(err.Error())
				continue
			}

			// Put the incoming Node on our message channel
			nodeJoinChan <- n
		}
	}()

	logger.Debug().Msg("UDP server listening")
	return nil
}

// Stop ...
func (u *udp) Stop() {
	u.isRunning = false
	u.server.Close()
	logger.Debug().Msg("UDP server stopped")
	waitGroup.Done()
}
