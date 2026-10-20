package tests

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	cliOnce sync.Once
	cliDir  string
	cliBin  string
	cliErr  error
)

// buildCLI builds the command line tool once for all tests and returns the
// binary and a directory the tests may write into.
func buildCLI(t *testing.T) (bin, dir string) {
	t.Helper()
	cliOnce.Do(func() {
		cliDir, cliErr = os.MkdirTemp("", "primitive-cli-")
		if cliErr != nil {
			return
		}
		cliBin = filepath.Join(cliDir, "primitive.exe")
		if out, err := exec.Command("go", "build", "-o", cliBin, "../cmd/primitive").CombinedOutput(); err != nil {
			cliErr = fmt.Errorf("build failed: %v\n%s", err, out)
		}
	})
	if cliErr != nil {
		t.Fatal(cliErr)
	}
	return cliBin, cliDir
}

func TestMain(m *testing.M) {
	code := m.Run()
	if cliDir != "" {
		os.RemoveAll(cliDir)
	}
	os.Exit(code)
}

// TestGoldenOutputs runs the real CLI with a fixed seed and one worker and
// compares the SVG hash with testdata/golden.txt. If the algorithm is changed
// on purpose, regenerate the file (see testdata/README.md).
func TestGoldenOutputs(t *testing.T) {
	bin, dir := buildCLI(t)
	f, err := os.Open("testdata/golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "|")
		if len(parts) != 3 {
			continue
		}
		name, args, want := parts[0], strings.Fields(parts[1]), parts[2]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := filepath.Join(dir, name+".svg")
			base := []string{"-i", "../examples/owl.png", "-o", out, "-n", "15", "-j", "1", "-seed", "5"}
			if strings.Contains(parts[1], "-algo quads") { // quads takes no seed or worker count
				base = []string{"-i", "../examples/owl.png", "-o", out, "-n", "300"}
			}
			if b, err := exec.Command(bin, append(base, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("run failed: %v\n%s", err, b)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != want {
				t.Fatalf("output changed for %q\n got  %s\n want %s", parts[1], got, want)
			}
		})
	}
}

func TestCLIRejectsBadInput(t *testing.T) {
	bin, dir := buildCLI(t)
	cases := map[string][]string{
		"bad palette":       {"-palette", "zzz", "-n", "2"},
		"bad quality":       {"-quality", "bogus"},
		"negative blur":     {"-blur", "-1", "-n", "2"},
		"no count":          {},
		"quads bad palette": {"-algo", "quads", "-palette", "zzz", "-n", "5"},
		"quads shape":       {"-algo", "quads", "-m", "3", "-n", "5"},
		"unknown algo":      {"-algo", "bogus", "-n", "5"},
		"bad cell":          {"-algo", "quads", "-cell", "hexagon", "-n", "5"},
		"border shapes":     {"-m", "1", "-border", "2", "-n", "5"},
		"bad border":        {"-algo", "quads", "-border", "-1", "-n", "5"},
	}
	for name, args := range cases {
		full := append([]string{"-i", "../examples/owl.png", "-o", filepath.Join(dir, "x.svg")}, args...)
		if err := exec.Command(bin, full...).Run(); err == nil {
			t.Errorf("%s: expected a non-zero exit", name)
		}
	}
}
