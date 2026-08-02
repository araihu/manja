package devlauncher

import "testing"

func TestParseDefaultsEnvironmentAndCLI(t *testing.T) {
	env := map[string]string{"MANJA_DEV_APP_PORT": "18080", "MANJA_DEV_PORT_RANGE": "20"}
	options, err := Parse([]string{"--app-port=19090", "--proxy-port", "17331", "--", "-spec", "fixture.yaml"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if options.Host != "127.0.0.1" || options.AppPort != 19090 || options.ProxyPort != 17331 || options.PortRange != 20 {
		t.Fatalf("options = %+v", options)
	}
	if len(options.ManjaArgs) != 2 || options.ManjaArgs[0] != "-spec" {
		t.Fatalf("manja args = %v", options.ManjaArgs)
	}
}

func TestParseRejectsInvalidPortsAndOverflow(t *testing.T) {
	for _, args := range [][]string{{"--app-port", "0"}, {"--proxy-port=65536"}} {
		if _, err := Parse(args, func(string) string { return "" }); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := Parse(nil, func(key string) string {
		if key == "MANJA_DEV_APP_PORT_BASE" {
			return "65530"
		}
		if key == "MANJA_DEV_PORT_RANGE" {
			return "10"
		}
		return ""
	}); err == nil {
		t.Fatal("overflow accepted")
	}
}
