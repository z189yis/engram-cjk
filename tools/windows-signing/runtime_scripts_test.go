package signing_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func script(t *testing.T, name string, directory string, env []string, args ...string) ([]byte, error) {
	t.Helper()
	filename, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", append([]string{filename}, args...)...)
	command.Dir = directory
	command.Env = append(os.Environ(), env...)
	return command.CombinedOutput()
}

func put(t *testing.T, directory, name, content string) string {
	t.Helper()
	filename := filepath.Join(directory, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestRebuiltWindowsBytesMustMatchCentralManifest(t *testing.T) {
	directory := t.TempDir()
	target := put(t, directory, "target.exe", "MZunsigned")
	put(t, directory, "engram-windows-amd64.exe", "MZsigned")
	manifest := fmt.Sprintf(`{"files":[{"name":"engram-windows-amd64.exe","unsignedSha256":"%x","signedSha256":"%x"}]}`, sha256.Sum256([]byte("MZunsigned")), sha256.Sum256([]byte("MZsigned")))
	put(t, directory, "signed-runtime-manifest.json", manifest)
	env := []string{"SIGNED_RUNTIME_DIRECTORY=" + directory, "RUNTIME_UNSIGNED_BUILD="}
	if output, err := script(t, "replace-signed-runtime.mjs", directory, env, target, "windows", "amd64"); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	bytes, err := os.ReadFile(target)
	if err != nil || string(bytes) != "MZsigned" {
		t.Fatalf("Signed bytes were not substituted: %v", err)
	}
	for _, testCase := range []struct{ name, contents string }{{"changed-rebuild", "MZchanged"}, {"already-signed", "MZsigned"}} {
		t.Run(testCase.name, func(t *testing.T) {
			put(t, directory, "target.exe", testCase.contents)
			if output, err := script(t, "replace-signed-runtime.mjs", directory, env, target, "windows", "amd64"); err == nil {
				t.Fatalf("Expected fail-closed mismatch: %s", output)
			}
		})
	}
	put(t, directory, "target.exe", "MZunsigned")
	put(t, directory, "engram-windows-amd64.exe", "MZtampered")
	if _, err := script(t, "replace-signed-runtime.mjs", directory, env, target, "windows", "amd64"); err == nil {
		t.Fatal("Tampered signed bytes accepted")
	}
	if _, err := script(t, "replace-signed-runtime.mjs", directory, []string{"SIGNED_RUNTIME_DIRECTORY=", "RUNTIME_UNSIGNED_BUILD="}, target, "windows", "amd64"); err == nil {
		t.Fatal("Missing signed runtime accepted")
	}
	if output, err := script(t, "replace-signed-runtime.mjs", directory, []string{"SIGNED_RUNTIME_DIRECTORY=", "RUNTIME_UNSIGNED_BUILD=1"}, target, "windows", "amd64"); err != nil {
		t.Fatalf("Explicit unsigned build failed: %v\n%s", err, output)
	}
}

func TestCollectRequiresExactlyBothWindowsBuildsInsideDist(t *testing.T) {
	directory := t.TempDir()
	put(t, directory, "dist/amd64/engram.exe", "MZamd64")
	put(t, directory, "dist/arm64/engram.exe", "MZarm64")
	valid := `[{"type":"Binary","goos":"windows","goarch":"amd64","path":"dist/amd64/engram.exe"},{"type":"Binary","goos":"windows","goarch":"arm64","path":"dist/arm64/engram.exe"}]`
	put(t, directory, "dist/artifacts.json", valid)
	if output, err := script(t, "collect-engram.mjs", directory, nil); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		bytes, err := os.ReadFile(filepath.Join(directory, "unsigned-runtime", "engram-windows-"+arch+".exe"))
		if err != nil || string(bytes) != "MZ"+arch {
			t.Fatalf("Missing collected %s: %v", arch, err)
		}
	}
	for _, invalid := range []string{`[]`, `[{"type":"Binary","goos":"windows","goarch":"amd64","path":"../escape.exe"}]`, `[{"type":"Binary","goos":"windows","goarch":"amd64","path":"dist/amd64/engram.exe"}]`} {
		put(t, directory, "dist/artifacts.json", invalid)
		if _, err := script(t, "collect-engram.mjs", directory, nil); err == nil {
			t.Fatal("Invalid build manifest accepted")
		}
	}
}
