package downloaders

import (
	"errors"
	"context"
	"net"
	"net/http"
	"fmt"
	"sync/atomic"
)

func getIP(ifaceName string) (ip net.IP, mask net.IPMask, err error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, nil, err
	}

	for _, addr := range addrs {
        switch v := addr.(type) {
        case *net.IPNet:
            ip = v.IP
			mask = v.Mask
        case *net.IPAddr:
            ip = v.IP
			mask = ip.DefaultMask()
        }

		if ip != nil && ip.To4() != nil {
			return ip, mask, nil
		}
	}
	return nil, nil, errors.New("No ip found")
}

// Returns the IP Address as v6 if available, else v4
func getNicIP(ip net.IP) (string, error) {
	addr := ip.String()
	if len(addr) != 0 {
		return addr, nil
	}
	return "", fmt.Errorf("No IPv4 or IPv6 address for Nic's IP's")
}	

func createHttpClient(ip net.IP) (*http.Client, error) {
	// Get our Client
	addr, nicErr := getNicIP(ip)
	if nicErr != nil {
		return nil, nicErr
	}

	// :0 tells linux to dynamically assign us an unused port
	// https://www.lifewire.com/port-0-in-tcp-and-udp-818145
	tcpAddr, resolveErr := net.ResolveTCPAddr("tcp", addr + ":0")
	if resolveErr != nil {
		return nil, resolveErr
	}

	// Configure how to connect to the NIC's address & ephemeral TCP port we've allocated
	dialer := &net.Dialer{LocalAddr: tcpAddr}
	dialContext := func(ctx context.Context, network, dailAddr string) (net.Conn, error) {
		conn, err := dialer.Dial(network, dailAddr)
		return conn, err
	}

	// Create HTTP client using our dialer
	transport := &http.Transport{DialContext: dialContext}
	client := &http.Client{
		Transport: transport,
	}
	return client, nil
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type MultiNicHTTPClient struct {
	// List of NICs we're load balancing traffic across
	NICs []net.IP 

	// HTTP Clients corresponding to said NICs
	httpClients []*http.Client

	//if this overflows a-ok as we'll start back at 0
	// only using it for shuffling traffic.
	counter uint32 
}

func NewMultiNicHTTPClient(nicNames []string) (*MultiNicHTTPClient, error) {
	mn := MultiNicHTTPClient{}
	mn.NICs = make([]net.IP, len(nicNames), len(nicNames))
	mn.httpClients = make([]*http.Client, len(nicNames), len(nicNames))

	// Get NicIPs & create httplients
	for i, nic := range nicNames {
		ip, _, err := getIP(nic)
		if err != nil {
			return nil, err
		}
		mn.NICs[i] = ip

		httpClient, err := createHttpClient(ip)
		if err != nil {
			return nil, err
		}

		mn.httpClients[i] = httpClient
	}	
	return &mn, nil
}

/**
 * From the go standard libraries http.Client docs:
 * The Client's Transport typically has internal state (cached TCP connections), so Clients should be reused instead of created as needed.
 * Clients are safe for concurrent use by multiple goroutines.
 */
func (mn *MultiNicHTTPClient) Client() (*http.Client) {
		// load balance across the NICs
		i := atomic.AddUint32(&mn.counter, 1) % uint32(len(mn.NICs))
		return mn.httpClients[i]
		
}

// Load balances traffic across the HTTP Clients
func (mn *MultiNicHTTPClient) Do(req *http.Request) (*http.Response, error) {
	client := mn.Client()
	return client.Do(req)
}