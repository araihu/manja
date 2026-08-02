package devlauncher

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultHost      = "127.0.0.1"
	defaultAppBase   = 8080
	defaultProxyBase = 7331
	defaultSiteBase  = 8180
	defaultRange     = 400
)

type Options struct {
	Host                         string
	AppPort, ProxyPort, SitePort int
	AppPortBase, ProxyPortBase   int
	SitePortBase, PortRange      int
	PrintPorts, Help             bool
	ManjaArgs                    []string
}

func Parse(args []string, getenv func(string) string) (Options, error) {
	options := Options{Host: valueOr(getenv("MANJA_DEV_HOST"), defaultHost), AppPortBase: defaultAppBase, ProxyPortBase: defaultProxyBase, SitePortBase: defaultSiteBase, PortRange: defaultRange}
	var err error
	for key, target := range map[string]*int{
		"MANJA_DEV_APP_PORT": &options.AppPort, "MANJA_DEV_PROXY_PORT": &options.ProxyPort, "MANJA_DEV_SITE_PORT": &options.SitePort,
		"MANJA_DEV_APP_PORT_BASE": &options.AppPortBase, "MANJA_DEV_PROXY_PORT_BASE": &options.ProxyPortBase, "MANJA_DEV_SITE_PORT_BASE": &options.SitePortBase,
	} {
		if value := getenv(key); value != "" {
			*target, err = parsePort(value, key)
			if err != nil {
				return Options{}, err
			}
		}
	}
	if value := getenv("MANJA_DEV_PORT_RANGE"); value != "" {
		options.PortRange, err = strconv.Atoi(value)
		if err != nil || options.PortRange < 1 {
			return Options{}, fmt.Errorf("MANJA_DEV_PORT_RANGE must be a positive integer")
		}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			options.ManjaArgs = append(options.ManjaArgs, args[index+1:]...)
			break
		}
		switch arg {
		case "--help", "-h":
			options.Help = true
		case "--print-ports":
			options.PrintPorts = true
		case "--host", "--app-port", "--proxy-port", "--site-port":
			if index+1 >= len(args) {
				return Options{}, fmt.Errorf("%s requires a value", arg)
			}
			index++
			if err := setOption(&options, arg, args[index]); err != nil {
				return Options{}, err
			}
		default:
			if strings.HasPrefix(arg, "--host=") {
				options.Host = strings.TrimPrefix(arg, "--host=")
			} else if key, value, ok := strings.Cut(arg, "="); ok && (key == "--app-port" || key == "--proxy-port" || key == "--site-port") {
				if err := setOption(&options, key, value); err != nil {
					return Options{}, err
				}
			} else {
				options.ManjaArgs = append(options.ManjaArgs, arg)
			}
		}
	}
	if options.Host == "" {
		return Options{}, fmt.Errorf("--host cannot be empty")
	}
	for name, base := range map[string]int{"app": options.AppPortBase, "proxy": options.ProxyPortBase, "site": options.SitePortBase} {
		if base+options.PortRange-1 > 65535 {
			return Options{}, fmt.Errorf("automatic %s port range exceeds 65535", name)
		}
	}
	return options, nil
}

func setOption(options *Options, key, value string) error {
	if key == "--host" {
		options.Host = value
		return nil
	}
	port, err := parsePort(value, key)
	if err != nil {
		return err
	}
	switch key {
	case "--app-port":
		options.AppPort = port
	case "--proxy-port":
		options.ProxyPort = port
	case "--site-port":
		options.SitePort = port
	}
	return nil
}

func parsePort(value, name string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be an integer TCP port between 1 and 65535", name)
	}
	return port, nil
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
