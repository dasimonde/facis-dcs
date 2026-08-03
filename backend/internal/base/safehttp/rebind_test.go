package safehttp

import (
	"net"
	"strings"
	"testing"
	"time"
)

// A name is resolved once and the connection is made to the address that was
// checked. Handing the name to the dialer let it resolve a second time, so a
// record whose answer changes between the two lookups passed the check on one
// address and connected to another.
func TestConnectionGoesToTheCheckedAddress(t *testing.T) {
	_, err := Client(2*time.Second, Policy{}).Get("http://localhost.:9/")
	if err == nil {
		t.Fatal("dialled a name resolving to loopback under the default policy")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

func TestAddressFormsThatEvadeTheObviousPredicates(t *testing.T) {
	for _, tc := range []struct {
		name string
		ip   string
	}{
		// Go's To4 reads ::ffff:127.0.0.1 but not the IPv4-compatible form, so
		// this reaches the check looking like a global IPv6 address.
		{"ipv4-compatible loopback", "::127.0.0.1"},
		{"ipv4-mapped loopback", "::ffff:127.0.0.1"},
		{"ipv4-compatible link-local", "::169.254.169.254"},
		// IsUnspecified only matches 0.0.0.0; Linux routes the whole /8 locally.
		{"this-network", "0.0.0.1"},
		{"broadcast", "255.255.255.255"},
		{"loopback", "127.0.0.1"},
		{"link-local", "169.254.169.254"},
		{"unspecified", "0.0.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("could not parse %s", tc.ip)
			}
			if err := permitted(ip, false); err == nil {
				t.Errorf("permitted %s", tc.ip)
			}
		})
	}

	// A private in-cluster address stays reachable: that is where peers live.
	for _, ok := range []string{"10.1.2.3", "192.168.4.5", "172.16.0.9", "93.184.216.34"} {
		if err := permitted(net.ParseIP(ok), false); err != nil {
			t.Errorf("refused %s, which a peer legitimately lives on: %v", ok, err)
		}
	}
}

func TestLoopbackAllowanceCoversItsAliases(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "::127.0.0.1"} {
		if err := permitted(net.ParseIP(ip), true); err != nil {
			t.Errorf("loopback %s refused although the policy allows loopback: %v", ip, err)
		}
	}
	if err := permitted(net.ParseIP("169.254.169.254"), true); err == nil || !strings.Contains(err.Error(), "link-local") {
		t.Errorf("allowing loopback must not allow link-local, got: %v", err)
	}
}
