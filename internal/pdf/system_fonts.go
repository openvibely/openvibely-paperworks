package pdf

import (
	"io"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
)

// fontFS supplies synthetic ancestors so PDFium can stat fonts/.. without
// escaping a preopened directory. Only the selected font directory is exposed.
type fontFS struct{ fonts fs.FS }

func (f fontFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	switch name {
	case ".":
		return &fontDirectory{name: ".", child: "usr"}, nil
	case "usr":
		return &fontDirectory{name: "usr", child: "share"}, nil
	case "usr/share":
		return &fontDirectory{name: "share", child: "fonts"}, nil
	case "usr/share/fonts":
		file, err := f.fonts.Open(".")
		if err != nil {
			return nil, err
		}
		dir, ok := file.(fs.ReadDirFile)
		if !ok {
			file.Close()
			return nil, fs.ErrInvalid
		}
		return fontRootDirectory{dir}, nil
	}
	if sub, ok := strings.CutPrefix(name, "usr/share/fonts/"); ok {
		return f.fonts.Open(sub)
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

type fontRootDirectory struct{ fs.ReadDirFile }

func (fontRootDirectory) Stat() (fs.FileInfo, error) { return fontDirInfo("fonts"), nil }

type fontDirectory struct {
	name, child string
	read        bool
}

func (d *fontDirectory) Close() error               { return nil }
func (d *fontDirectory) Read([]byte) (int, error)   { return 0, fs.ErrInvalid }
func (d *fontDirectory) Stat() (fs.FileInfo, error) { return fontDirInfo(d.name), nil }
func (d *fontDirectory) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.read {
		if n > 0 {
			return nil, io.EOF
		}
		return nil, nil
	}
	d.read = true
	return []fs.DirEntry{fs.FileInfoToDirEntry(fontDirInfo(d.child))}, nil
}

type fontDirInfo string

func (i fontDirInfo) Name() string     { return string(i) }
func (fontDirInfo) Size() int64        { return 0 }
func (fontDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0555 }
func (fontDirInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (fontDirInfo) IsDir() bool        { return true }
func (fontDirInfo) Sys() any           { return nil }

func systemFontFS() wazero.FSConfig {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = "/System/Library/Fonts/Supplemental"
	case "linux":
		dir = "/usr/share/fonts"
	case "windows":
		if os.Getenv("WINDIR") == "" {
			return wazero.NewFSConfig()
		}
		dir = os.Getenv("WINDIR") + `\Fonts`
	default:
		return wazero.NewFSConfig()
	}
	// Root prevents symlinks in the font directory from exposing other host files.
	// Its lifetime matches the process-wide PDFium pool.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return wazero.NewFSConfig()
	}
	return wazero.NewFSConfig().WithFSMount(fontFS{root.FS()}, "/")
}
