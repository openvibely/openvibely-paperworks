package office

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type faceKey struct {
	family       string
	bold, italic bool
}

var (
	faceMu    sync.Mutex
	faceCache = map[faceKey]*sfnt.Font{}
	fontFiles []string
	fontOnce  sync.Once
)

func systemFontFiles() []string {
	fontOnce.Do(func() {
		roots := []string{"/System/Library/Fonts", "/Library/Fonts", "/usr/share/fonts", "/usr/local/share/fonts"}
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, filepath.Join(home, "Library/Fonts"), filepath.Join(home, ".fonts"), filepath.Join(home, ".local/share/fonts"))
		}
		if windir := os.Getenv("WINDIR"); windir != "" {
			roots = append(roots, filepath.Join(windir, "Fonts"))
		}
		for _, root := range roots {
			filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					switch strings.ToLower(filepath.Ext(p)) {
					case ".ttf", ".otf", ".ttc":
						fontFiles = append(fontFiles, p)
					}
				}
				return nil
			})
		}
	})
	return fontFiles
}

func loadFonts(path string) []*sfnt.Font {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if c, err := sfnt.ParseCollection(raw); err == nil {
		var out []*sfnt.Font
		for i := 0; i < c.NumFonts(); i++ {
			if f, err := c.Font(i); err == nil {
				out = append(out, f)
			}
		}
		return out
	}
	if f, err := sfnt.Parse(raw); err == nil {
		return []*sfnt.Font{f}
	}
	return nil
}

func faceMatches(f *sfnt.Font, k faceKey) bool {
	var b sfnt.Buffer
	family, err := f.Name(&b, sfnt.NameIDFamily)
	if err != nil || !strings.EqualFold(family, k.family) {
		return false
	}
	style, _ := f.Name(&b, sfnt.NameIDSubfamily)
	style = strings.ToLower(style)
	return strings.Contains(style, "bold") == k.bold && (strings.Contains(style, "italic") || strings.Contains(style, "oblique")) == k.italic
}

// FindFont matches an installed font by exact family and face; it never
// substitutes a different family.
func FindFont(family string, bold, italic bool) *sfnt.Font {
	k := faceKey{family, bold, italic}
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faceCache[k]; ok {
		return f
	}
	var candidates []string
	if fc, err := exec.LookPath("fc-match"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pattern := family
		if bold {
			pattern += ":style=Bold"
		}
		if italic {
			pattern += " Italic"
		}
		if out, err := exec.CommandContext(ctx, fc, "-f", "%{file}", pattern).Output(); err == nil {
			candidates = append(candidates, string(out))
		}
		cancel()
	}
	candidates = append(candidates, systemFontFiles()...)
	for _, p := range candidates {
		for _, f := range loadFonts(p) {
			if faceMatches(f, k) {
				faceCache[k] = f
				return f
			}
		}
	}
	faceCache[k] = nil
	return nil
}

// Advance returns the advance width of s in points at size pt.
func Advance(f *sfnt.Font, s string, size float64) float64 {
	var b sfnt.Buffer
	ppem := fixed.Int26_6(size * 64)
	total := fixed.Int26_6(0)
	for _, r := range s {
		g, err := f.GlyphIndex(&b, r)
		if err != nil {
			continue
		}
		a, err := f.GlyphAdvance(&b, g, ppem, font.HintingNone)
		if err == nil {
			total += a
		}
	}
	return float64(total) / 64
}
