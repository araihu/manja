package devlauncher

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func AirArgs(options Options, ports Ports) []string {
	entry := strings.Join(append([]string{"./tmp/manja-dev", "-addr", fmt.Sprintf("%s:%d", options.Host, ports.AppPort), "-data-dir", ".manja/data"}, options.ManjaArgs...), ",")
	return []string{"run", "github.com/air-verse/air@v1.65.3", "-c", ".air.toml", "--build.entrypoint", entry, "--proxy.app_port", strconv.Itoa(ports.AppPort), "--proxy.proxy_port", strconv.Itoa(ports.ProxyPort)}
}

func SiteArgs(options Options, ports Ports) []string {
	return []string{"run", "./cmd/server", "-addr", fmt.Sprintf("%s:%d", options.Host, ports.SitePort)}
}

func Run(ctx context.Context, root string, options Options, ports Ports, output io.Writer) error {
	appURL := fmt.Sprintf("http://%s:%d", options.Host, ports.AppPort)
	proxyURL := fmt.Sprintf("http://%s:%d", options.Host, ports.ProxyPort)
	siteURL := fmt.Sprintf("http://%s:%d", options.Host, ports.SitePort)
	fmt.Fprintf(output, "Manja app: %s\nAir reload proxy: %s\nManja site: %s\nOpen %s\n", appURL, proxyURL, siteURL, siteURL)
	env := append(os.Environ(), "MANJA_DEV_APP_URL="+appURL, "MANJA_DEV_PROXY_URL="+proxyURL, "MANJA_DEV_SITE_URL="+siteURL)
	air := exec.Command("go", AirArgs(options, ports)...)
	air.Dir = root
	air.Env = env
	air.Stdout, air.Stderr = output, output
	site := exec.Command("go", SiteArgs(options, ports)...)
	site.Dir = root + "/site"
	site.Env = env
	site.Stdout, site.Stderr = output, output
	if err := air.Start(); err != nil {
		return err
	}
	if err := site.Start(); err != nil {
		_ = air.Process.Kill()
		_ = air.Wait()
		return err
	}
	type result struct {
		process *os.Process
		err     error
	}
	done := make(chan result, 2)
	go func() { done <- result{air.Process, air.Wait()} }()
	go func() { done <- result{site.Process, site.Wait()} }()
	var first result
	select {
	case first = <-done:
	case <-ctx.Done():
		first = result{nil, ctx.Err()}
	}
	for _, process := range []*os.Process{air.Process, site.Process} {
		if process != first.process {
			_ = process.Signal(os.Interrupt)
		}
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		for _, process := range []*os.Process{air.Process, site.Process} {
			_ = process.Kill()
		}
	}
	return first.err
}
