package sonda

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTargets(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, File), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadReadsTargets(t *testing.T) {
	dir := t.TempDir()
	writeTargets(t, dir, `
[[target]]
name = "api"
dir = "cmd/api"
build = ["make", "build"]
run = ["./api", "--port", "8080"]
env = { REVIEW_TRACE = "1" }

[[target]]
name = "worker"
run = ["go", "run", "./cmd/worker"]
`)
	targets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets", len(targets))
	}
	api := targets[0]
	if api.Name != "api" || api.Dir != filepath.Join(dir, "cmd/api") {
		t.Errorf("api = %+v", api)
	}
	if strings.Join(api.Build, " ") != "make build" || strings.Join(api.Run, " ") != "./api --port 8080" {
		t.Errorf("api commands = %v %v", api.Build, api.Run)
	}
	if api.Env["REVIEW_TRACE"] != "1" {
		t.Errorf("api env = %v", api.Env)
	}
	worker := targets[1]
	if worker.Dir != dir || worker.Build != nil {
		t.Errorf("worker = %+v", worker)
	}
}

func TestADirectoryWithNoFileDeclaresNothing(t *testing.T) {
	targets, err := Load(t.TempDir())
	if err != nil || targets != nil {
		t.Errorf("got %v, %v", targets, err)
	}
}

func TestAFileThatCannotBeReadNamesItself(t *testing.T) {
	for _, body := range []string{
		"[[target]]\nname = \"x\"\n",                                // no run command
		"[[target]]\nrun = [\"x\"]\n",                               // no name
		"[[target]]\nname = 1\nrun = [\"x\"]\n",                     // wrong type
		"[[target]]\nname = \"x\"\nrun = [\"x\"]\nruns = [\"x\"]\n", // misspelt key
		"not toml at all",
	} {
		dir := t.TempDir()
		writeTargets(t, dir, body)
		_, err := Load(dir)
		if err == nil {
			t.Errorf("%q loaded without complaint", body)
			continue
		}
		if !strings.HasPrefix(err.Error(), File) {
			t.Errorf("%q: error %q does not name the file", body, err)
		}
	}
}

func TestRelocateKeepsThePathUnderTheRoot(t *testing.T) {
	tgt := Target{Name: "api", Dir: "/repo/cmd/api"}
	moved, err := tgt.Relocate("/repo", "/cache/repo-1234")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Dir != "/cache/repo-1234/cmd/api" {
		t.Errorf("dir = %s", moved.Dir)
	}
	if _, err := (Target{Name: "x", Dir: "/elsewhere"}).Relocate("/repo", "/cache"); err == nil {
		t.Error("a target outside the repository was relocated")
	}
}
