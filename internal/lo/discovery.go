package lo

import (
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

// Only completed, hashed OV package directories are candidates. Do not search
// arbitrary descendant files or other platforms' binaries.
func findCachedOffice(cache, platform, arch string, probe func(string) bool) (string, error) {
	root := filepath.Join(cache, "openvibely", "skill-tools", "libreoffice")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", ErrMissing
	}
	type release struct {
		name    string
		version *semver.Version
	}
	var releases []release
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := semver.StrictNewVersion(e.Name())
		if err == nil {
			releases = append(releases, release{e.Name(), v})
		}
	}
	sort.Slice(releases, func(i, j int) bool {
		c := releases[i].version.Compare(releases[j].version)
		if c == 0 {
			return releases[i].name < releases[j].name
		}
		return c > 0
	})
	var relative []string
	switch platform {
	case "darwin":
		relative = []string{"LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		relative = []string{"program/soffice.exe", "LibreOffice/program/soffice.exe"}
	case "linux":
		relative = []string{"program/soffice", "lib/libreoffice/program/soffice", "usr/lib/libreoffice/program/soffice"}
	default:
		return "", ErrMissing
	}
	for _, v := range releases {
		dir := filepath.Join(root, v.name, platform, arch)
		packages, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, pkg := range packages {
			hash, err := hex.DecodeString(pkg.Name())
			if !pkg.IsDir() || err != nil || len(hash) != 32 {
				continue
			}
			for _, rel := range relative {
				candidate := filepath.Join(dir, pkg.Name(), filepath.FromSlash(rel))
				st, err := os.Stat(candidate)
				if err != nil || !st.Mode().IsRegular() {
					continue
				}
				if platform != "windows" && st.Mode().Perm()&0111 == 0 {
					continue
				}
				if probe(candidate) {
					return candidate, nil
				}
			}
		}
	}
	return "", ErrMissing
}

func probeCachedOffice(exe string) bool {
	tmp, err := os.MkdirTemp("", "paperworks-lo-discovery-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-env:UserInstallation="+FileURL(filepath.Join(tmp, "profile")), "--headless", "--version")
	cmd.Env = isolatedEnv(tmp)
	out, err := cmd.Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "LibreOffice ")
}
