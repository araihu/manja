package devlauncher

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"path/filepath"
)

type Ports struct{ AppPort, ProxyPort, SitePort int }

func ChoosePorts(ctx context.Context, options Options, worktree string) (Ports, error) {
	if options.PortRange == 0 {
		options.PortRange = defaultRange
	}
	if options.AppPort != 0 && options.ProxyPort != 0 && options.SitePort != 0 {
		if options.AppPort == options.ProxyPort || options.AppPort == options.SitePort || options.ProxyPort == options.SitePort {
			return Ports{}, fmt.Errorf("app, proxy, and site ports must be different")
		}
		for _, port := range []int{options.AppPort, options.ProxyPort, options.SitePort} {
			if !canListen(ctx, options.Host, port) {
				return Ports{}, fmt.Errorf("port %d is already in use on %s", port, options.Host)
			}
		}
		return Ports{options.AppPort, options.ProxyPort, options.SitePort}, nil
	}
	real, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		return Ports{}, err
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(real))
	offset := int(hash.Sum32()) % options.PortRange
	for index := 0; index < options.PortRange; index++ {
		candidate := (offset + index) % options.PortRange
		ports := Ports{choose(options.AppPort, options.AppPortBase, candidate), choose(options.ProxyPort, options.ProxyPortBase, candidate), choose(options.SitePort, options.SitePortBase, candidate)}
		if ports.AppPort == ports.ProxyPort || ports.AppPort == ports.SitePort || ports.ProxyPort == ports.SitePort {
			continue
		}
		if availableOrFixed(ctx, options.Host, ports.AppPort, options.AppPort) && availableOrFixed(ctx, options.Host, ports.ProxyPort, options.ProxyPort) && availableOrFixed(ctx, options.Host, ports.SitePort, options.SitePort) {
			return ports, nil
		}
	}
	return Ports{}, fmt.Errorf("no available app/proxy/site port set found")
}

func choose(fixed, base, offset int) int {
	if fixed != 0 {
		return fixed
	}
	return base + offset
}
func availableOrFixed(ctx context.Context, host string, candidate, fixed int) bool {
	if fixed != 0 {
		return canListen(ctx, host, candidate)
	}
	return canListen(ctx, host, candidate)
}
func canListen(ctx context.Context, host string, port int) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
