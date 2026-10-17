package lo

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReferenceIsolatedProfileAndEnvironment(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	originalEnv := os.Environ()
	dir := t.TempDir()
	src := filepath.Join(dir, "input.xlsx")
	dst := filepath.Join(dir, "output.xlsx")
	os.WriteFile(src, []byte("unchanged original"), 0600)
	var profile string
	calls := 0
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		if !strings.Contains(cmd.Args[1], "-env:UserInstallation=file:") {
			t.Fatal(cmd.Args)
		}
		u, err := url.Parse(strings.TrimPrefix(cmd.Args[1], "-env:UserInstallation="))
		if err != nil {
			t.Fatal(err)
		}
		profile = u.Path
		config, err := os.ReadFile(filepath.Join(profile, "user", "registrymodifications.xcu"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"MacroSecurityLevel", "DisableMacrosExecution", "<value>true</value>"} {
			if !bytes.Contains(config, []byte(want)) {
				t.Fatal("security configuration missing", want)
			}
		}
		env := map[string]string{}
		for _, v := range cmd.Env {
			k, v, ok := strings.Cut(v, "=")
			if ok {
				env[k] = v
			}
		}
		if env["HOME"] != os.Getenv("HOME") {
			t.Fatal("HOME changed")
		}
		for _, k := range []string{"XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
			st, e := os.Stat(env[k])
			if e != nil || !st.IsDir() || env[k] == os.Getenv(k) {
				t.Fatal(k, e)
			}
		}
		if calls == 1 {
			d := filepath.Join(profile, "user", "basic", "Standard")
			os.MkdirAll(d, 0700)
			return nil, os.WriteFile(filepath.Join(d, "script.xlb"), []byte("library"), 0600)
		}
		if env["PW_IN"] != FileURL(src) || env["PW_OUT"] != FileURL(dst) || env["PW_WHOLE_SHEETS"] != "" {
			t.Fatal(env["PW_IN"], env["PW_OUT"])
		}
		macro, e := os.ReadFile(filepath.Join(profile, "user", "basic", "Standard", "Module1.xba"))
		if e != nil || !bytes.Contains(macro, []byte("calculateAll")) {
			t.Fatal("macro unavailable", e)
		}
		if e = os.WriteFile(dst, []byte("distinct calculated artifact"), 0600); e != nil {
			t.Fatal(e)
		}
		return nil, os.WriteFile(env["PW_STATUS"], []byte("ok"), 0600)
	})
	ctx := context.WithValue(context.Background(), commandRunnerKey{}, runner)
	if err = Refresh(ctx, src, dst, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	if _, err = os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("profile not removed", err)
	}
	if !reflect.DeepEqual(os.Environ(), originalEnv) {
		t.Fatal("process environment changed")
	}
	data, _ := os.ReadFile(src)
	if string(data) != "unchanged original" {
		t.Fatal("source changed")
	}
	data, _ = os.ReadFile(dst)
	if string(data) != "distinct calculated artifact" {
		t.Fatal("missing distinct output")
	}
}

func TestFailedOfficeProcessCannotPublishSuccess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	dst := filepath.Join(t.TempDir(), "output.xlsx")
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		u, _ := url.Parse(strings.TrimPrefix(cmd.Args[1], "-env:UserInstallation="))
		if cmd.Args[len(cmd.Args)-1] == "--terminate_after_init" {
			d := filepath.Join(u.Path, "user", "basic", "Standard")
			os.MkdirAll(d, 0700)
			return nil, os.WriteFile(filepath.Join(d, "script.xlb"), []byte("library"), 0600)
		}
		for _, v := range cmd.Env {
			if strings.HasPrefix(v, "PW_STATUS=") {
				os.WriteFile(strings.TrimPrefix(v, "PW_STATUS="), []byte("ok"), 0600)
			}
		}
		os.WriteFile(dst, []byte("partial"), 0600)
		return []byte("failed after status write"), errors.New("process failed")
	})
	err = Refresh(context.WithValue(context.Background(), commandRunnerKey{}, runner), "input.xlsx", dst, false)
	if err == nil || !strings.Contains(err.Error(), "process failed") {
		t.Fatal(err)
	}
	if _, err = os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("failed output published", err)
	}
}

// Original command-only environment contract, separate from Refresh's two calls.
func TestReferenceCommandEnvironmentIsolation(t *testing.T) {
	environment := func(values []string) map[string]string {
		result := make(map[string]string)
		for _, value := range values {
			if key, value, ok := strings.Cut(value, "="); ok {
				result[key] = value
			}
		}
		return result
	}
	original := environment(os.Environ())
	root := t.TempDir()
	// Reference mocks executable lookup; the runner below intercepts execution.
	s := &session{exe: "/fixture/soffice", tmp: root, profile: filepath.Join(root, "profile")}
	calls := 0
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		env := environment(cmd.Env)
		home, present := env["HOME"]
		oldHome, oldPresent := original["HOME"]
		if home != oldHome || present != oldPresent {
			t.Fatal("HOME changed")
		}
		for _, key := range []string{"XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
			info, err := os.Stat(env[key])
			if err != nil || !info.IsDir() {
				t.Fatal(key, err)
			}
		}
		if cache, present := original["XDG_CACHE_HOME"]; present && env["XDG_CACHE_HOME"] == cache {
			t.Fatal("cache directory not isolated")
		}
		return nil, nil
	})
	ctx := context.WithValue(context.Background(), commandRunnerKey{}, runner)
	if err := s.run(ctx, nil, "--headless"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(environment(os.Environ()), original) {
		t.Fatal("process environment changed")
	}
	// Supplemental guard: absence of a callback cannot satisfy its assertions.
	if calls != 1 {
		t.Fatal("command callback count", calls)
	}
}
