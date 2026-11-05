package lo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type executableIdentity struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	ModTimeNS int64  `json:"mod_time_ns"`
	Mode      uint32 `json:"mode"`
	FileID    string `json:"file_id"`
	SHA256    string `json:"sha256"`
}

type versionCacheEntry struct {
	Executable executableIdentity `json:"executable"`
	Version    string             `json:"version"`
}

func identifyExecutable(exe string) (executableIdentity, error) {
	path, err := filepath.Abs(exe)
	if err != nil {
		return executableIdentity{}, err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	info, err := os.Stat(path)
	if err != nil {
		return executableIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return executableIdentity{}, os.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return executableIdentity{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return executableIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return executableIdentity{}, os.ErrInvalid
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return executableIdentity{}, err
	}
	return executableIdentity{
		Path:      path,
		Size:      info.Size(),
		ModTimeNS: info.ModTime().UnixNano(),
		Mode:      uint32(info.Mode()),
		FileID:    nativeFileIdentity(info),
		SHA256:    hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func nativeFileIdentity(info os.FileInfo) string {
	value := reflect.ValueOf(info.Sys())
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}
	var identity strings.Builder
	for _, name := range []string{"Dev", "Ino", "VolumeSerialNumber", "FileIndexHigh", "FileIndexLow", "CreationTime"} {
		field := value.FieldByName(name)
		if !field.IsValid() || !field.CanInterface() {
			continue
		}
		if identity.Len() > 0 {
			identity.WriteByte(';')
		}
		fmt.Fprintf(&identity, "%s=%v", name, field.Interface())
	}
	return identity.String()
}

func versionCachePath(root, executable string) string {
	key := sha256.Sum256([]byte(executable))
	return filepath.Join(root, "openvibely", "paperworks", "soffice-version", hex.EncodeToString(key[:])+".json")
}

func readVersionCache(root string, identity executableIdentity) (string, bool) {
	data, err := os.ReadFile(versionCachePath(root, identity.Path))
	if err != nil {
		return "", false
	}
	var entry versionCacheEntry
	if json.Unmarshal(data, &entry) != nil || entry.Executable != identity {
		return "", false
	}
	version := strings.TrimSpace(entry.Version)
	if version == "" {
		return "", false
	}
	return version, true
}

func writeVersionCache(root string, identity executableIdentity, version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	path := versionCachePath(root, identity.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(versionCacheEntry{Executable: identity, Version: version})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".soffice-version-*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// Windows does not replace an existing destination with Rename.
		if os.Remove(path) != nil || os.Rename(tmpPath, path) != nil {
			return
		}
	}
}
