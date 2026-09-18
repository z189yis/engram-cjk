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
	if !strings.Contains(string(config), "tools/windows-signing/replace-signed-runtime.mjs") {
		t.Fatal("Windows release must substitute verified signed bytes before archives, checksums, and SBOMs")
	}
	workflow, err := os.ReadFile("../../.github/workflows/fork-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"windows-latest", "environment: release", "repository: rongxinzy/RongxinAI", "verify-return.mjs", "Assert-WindowsRuntimeSignature", "--skip=publish,announce", "GORELEASER_CURRENT_TAG:", "publish-engram.mjs"} {
		if !strings.Contains(string(workflow), required) {
			t.Fatalf("Missing release boundary: %s", required)
		}
	}
	text := string(workflow)
	build := text[strings.Index(text, "  build:"):strings.Index(text, "  publish:")]
	if !strings.Contains(build, "needs: verify") || !strings.Contains(build, "runs-on: windows-latest") {
		t.Fatal("Unsigned and publication rebuilds must use the same host platform after Linux source verification")
	}
	if strings.Contains(text, "CERTUM_") || strings.Contains(text, "setup-certum-signing") {
		t.Fatal("Signing credentials must remain exclusively in RongxinAI")
	}
	if strings.Index(text, "cmp ") > strings.Index(text, "run: node tools/windows-signing/publish-engram.mjs") {
		t.Fatal("Archive validation must precede release publication")
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
