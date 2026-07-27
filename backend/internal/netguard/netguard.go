// Package netguard restricts where outbound scan traffic may connect.
//
// Every target Omastx dials is derived from untrusted input: the API server URL
// in an uploaded kubeconfig, and the chart repository URLs and image references
// read out of a connected cluster. Without a guard, an authenticated user can
// aim the backend at addresses that are never real scan targets and use it as a
// proxy into the network it runs in - most damagingly the cloud instance
// metadata service, which hands out the node's IAM credentials to anything that
// asks (SSRF, SPEC §2.6).
//
// The check runs from the dialer's Control hook, so it sees the resolved IP
// immediately before connect. A hostname that resolves to a blocked address is
// therefore refused as well, which is what makes DNS rebinding pointless.
// Private ranges stay reachable on purpose: on-prem API servers, internal Harbor
// or Nexus registries, and VPN-routed clusters are the normal case.
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"
)

// allowLoopback is deployment-wide policy rather than a per-call option: only
// local development points Omastx at a cluster on localhost (kind, minikube).
var allowLoopback atomic.Bool

// AllowLoopback permits connections to loopback addresses. Call once at startup
// with the dev-mode flag; production must leave it off.
func AllowLoopback(allow bool) { allowLoopback.Store(allow) }

// imdsIPv6 is the IPv6 instance metadata address. It sits inside the unique-local
// range that stays allowed for on-prem targets, so it has to be named.
var imdsIPv6 = net.ParseIP("fd00:ec2::254")

// checkAddr reports why address must not be dialed, or nil when it may be.
// address is "ip:port" as resolved by the dialer.
func checkAddr(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("refusing to connect to %q: unrecognised address", address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("refusing to connect to %q: not an IP address", host)
	}
	// Collapse IPv4-mapped IPv6 (::ffff:169.254.169.254) onto its IPv4 form so
	// one spelling of an address can't dodge the checks below.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	switch {
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return blocked(ip, "link-local addresses host the cloud metadata service and are never a cluster, "+
			"registry, or chart repository")
	case ip.Equal(imdsIPv6):
		return blocked(ip, "this is the cloud metadata service")
	case ip.IsUnspecified(), ip.IsMulticast(), ip.IsInterfaceLocalMulticast():
		return blocked(ip, "the address is not a routable host")
	case ip.IsLoopback() && !allowLoopback.Load():
		return blocked(ip, "loopback addresses reach Omastx itself rather than a cluster")
	}
	return nil
}

func blocked(ip net.IP, why string) error {
	return fmt.Errorf("refusing to connect to %s: %s. Use the target's routable address", ip, why)
}

func control(_, address string, _ syscall.RawConn) error { return checkAddr(address) }

// dialer is safe for concurrent use, so one instance serves every caller.
var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	Control:   control,
}

// DialContext dials address only if it passes the guard. It is shaped for
// rest.Config.Dial and http.Transport.DialContext.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return dialer.DialContext(ctx, network, address)
}

// Transport clones http.DefaultTransport with the guard installed, keeping the
// stdlib's pooling and proxy behaviour.
func Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = DialContext
	return t
}
