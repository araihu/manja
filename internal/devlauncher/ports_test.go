package devlauncher

import (
	"context"
	"net"
	"testing"
)

func TestChoosePortsHonorsExplicitDistinctPorts(t *testing.T) {
	ports := reserveThreePorts(t)
	selected, err := ChoosePorts(context.Background(), Options{Host: "127.0.0.1", AppPort: ports[0], ProxyPort: ports[1], SitePort: ports[2], PortRange: 10}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if selected.AppPort != ports[0] || selected.ProxyPort != ports[1] || selected.SitePort != ports[2] {
		t.Fatalf("ports = %+v", selected)
	}
}

func TestChoosePortsRejectsCollision(t *testing.T) {
	port := reserveThreePorts(t)[0]
	_, err := ChoosePorts(context.Background(), Options{Host: "127.0.0.1", AppPort: port, ProxyPort: port, SitePort: port, PortRange: 10}, t.TempDir())
	if err == nil {
		t.Fatal("collision accepted")
	}
}

func reserveThreePorts(t *testing.T) [3]int {
	t.Helper()
	var result [3]int
	for index := range result {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		result[index] = listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
	}
	return result
}
