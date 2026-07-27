package architecture_test

import (
	"testing"
)

func TestProjectionWasmBuild(t *testing.T) {
	cmd := command(repositoryRoot(t), "go", "build", "-trimpath", "./application/projection")
	cmd.Env = append(cmd.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		t.Fatalf("compile projection for js/wasm: %v\n%s", err, output)
	}
}
