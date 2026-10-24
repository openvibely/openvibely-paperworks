package cli

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewLockProcess(t *testing.T) {
	if os.Getenv("PAPERWORKS_TEST_PREVIEW_LOCK") != "1" {
		return
	}
	release, err := lockSlidePreview(os.Getenv("PAPERWORKS_TEST_PREVIEW_OUT"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	fmt.Println("locked")
	_, _ = os.Stdin.Read(make([]byte, 1))
}

func TestPreviewPublicationCrossProcessLock(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "preview")
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "slide-2.png"), []byte("previous second slide"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPreviewLockProcess$")
	cmd.Env = append(os.Environ(), "PAPERWORKS_TEST_PREVIEW_LOCK=1", "PAPERWORKS_TEST_PREVIEW_OUT="+out)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("child lock: %q %v", line, err)
	}
	stage := filepath.Join(dir, "stage")
	if err := os.Mkdir(stage, 0755); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(stage, "page.png")
	if err := writePreview(png, image.NewRGBA(image.Rect(0, 0, 2, 2)), "png"); err != nil {
		t.Fatal(err)
	}
	publish := func() error {
		_, err := publishSelectedSlidePreview(context.Background(), stage, out, []string{png}, []map[string]any{{"slide": 1}}, map[int]bool{1: true})
		return err
	}
	if err := publish(); err == nil || !strings.Contains(err.Error(), "publication is locked") {
		t.Fatalf("expected lock rejection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "slide-1.png")); !os.IsNotExist(err) {
		t.Fatal("blocked publisher changed output")
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := publish(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(out, "slide-2.png")); err != nil || string(b) != "previous second slide" {
		t.Fatal("unselected slide lost")
	}
	// Successful publication released its lock.
	release, err := lockSlidePreview(out)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
