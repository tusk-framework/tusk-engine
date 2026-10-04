package control

import "testing"

func TestLoopbackAddressDefaultsToLocal(t *testing.T) {
	if !isLoopbackAddress("") {
		t.Fatal("empty address should resolve to loopback")
	}
	if !isLoopbackAddress("127.0.0.1") {
		t.Fatal("IPv4 loopback should be local")
	}
	if !isLoopbackAddress("::1") {
		t.Fatal("IPv6 loopback should be local")
	}
	if isLoopbackAddress("0.0.0.0") {
		t.Fatal("wildcard address must not be treated as loopback")
	}
}
