package signing_test

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestForkReleaseSignsWindowsBeforeArchiving(t *testing.T) {
	config, err := os.ReadFile("../../.goreleaser.fork.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "tools/windows-signing/sign-runtime.ps1") {
		t.Fatal("Windows release must sign executable bytes in a build post-hook before archives, checksums, and SBOMs")
	}
	workflow, err := os.ReadFile("../../.github/workflows/fork-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"windows-latest", "environment: release", "setup-certum-signing", "CERTUM_CERT_THUMBPRINT:", "needs: verify"} {
		if !strings.Contains(string(workflow), required) {
			t.Fatalf("Missing release boundary: %s", required)
		}
	}
}

func TestAuthenticodePolicy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell Authenticode policy tests run on Windows CI")
	}
	output, err := exec.Command("powershell.exe", "-NoProfile", "-File", "runtime-authenticode.test.ps1").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	t.Log(string(output))
}
