package tests

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildCLI(t *testing.T) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "primitive.exe")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/primitive").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin, dir
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
		"bad palette":   {"-palette", "zzz", "-n", "2"},
		"bad quality":   {"-quality", "bogus"},
		"negative blur": {"-blur", "-1", "-n", "2"},
		"no count":      {},
		"quads palette": {"-algo", "quads", "-palette", "#fff", "-n", "5"},
		"quads shape":   {"-algo", "quads", "-m", "3", "-n", "5"},
		"unknown algo":  {"-algo", "bogus", "-n", "5"},
		"bad cell":      {"-algo", "quads", "-cell", "triangle", "-n", "5"},
		"border shapes": {"-m", "1", "-border", "2", "-n", "5"},
		"bad border":    {"-algo", "quads", "-border", "-1", "-n", "5"},
	}
	for name, args := range cases {
		full := append([]string{"-i", "../examples/owl.png", "-o", filepath.Join(dir, "x.svg")}, args...)
		if err := exec.Command(bin, full...).Run(); err == nil {
			t.Errorf("%s: expected a non-zero exit", name)
		}
	}
}
