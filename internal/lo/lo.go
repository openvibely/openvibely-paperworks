// Package lo drives a local LibreOffice installation for conversion,
// recalculation and field/index refresh. Every run uses a throwaway profile
// with document macros disabled; no Java or Reference bridge is required.
package lo

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

//go:embed refresh.bas
var refreshMacro string

// ErrMissing reports that LibreOffice could not be located.
var ErrMissing = errors.New("LibreOffice (soffice) not found: install it or set DOCUMENT_SOFFICE")

const runTimeout = 180 * time.Second

// Find locates soffice via DOCUMENT_SOFFICE, PATH and standard install paths.
func Find() (string, error) {
	if p := os.Getenv("DOCUMENT_SOFFICE"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("DOCUMENT_SOFFICE=%s is not a file", p)
	}
	for _, n := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	var cands []string
	switch runtime.GOOS {
	case "darwin":
		cands = []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if v := os.Getenv(env); v != "" {
				cands = append(cands, filepath.Join(v, "LibreOffice", "program", "soffice.exe"))
			}
		}
	default:
		cands = []string{"/usr/lib/libreoffice/program/soffice", "/opt/libreoffice/program/soffice", "/usr/bin/soffice"}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", ErrMissing
}

// Version returns the `soffice --version` line, or "" when unavailable.
func Version(ctx context.Context) string {
	exe, err := Find()
	if err != nil {
		return ""
	}
	tmp, err := os.MkdirTemp("", "paperworks-lo-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-env:UserInstallation="+FileURL(filepath.Join(tmp, "profile")), "--version")
	cmd.Env = isolatedEnv(tmp)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FileURL converts a local path to a file:// URL accepted by LibreOffice.
func FileURL(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}

func isolatedEnv(tmp string) []string {
	env := os.Environ()
	for _, k := range []string{"XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
		d := filepath.Join(tmp, strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(k, "XDG_"), "_HOME")))
		os.MkdirAll(d, 0o700)
		env = append(env, k+"="+d)
	}
	return env
}

const securityXCU = `<?xml version="1.0" encoding="UTF-8"?>
<oor:items xmlns:oor="http://openoffice.org/2001/registry" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="MacroSecurityLevel" oor:op="fuse"><value>3</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="DisableMacrosExecution" oor:op="fuse"><value>true</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Misc"><prop oor:name="UseLocking" oor:op="fuse"><value>false</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Misc"><prop oor:name="FirstRun" oor:op="fuse"><value>false</value></prop></item>
<item oor:path="/org.openoffice.Setup/Office"><prop oor:name="ooSetupInstCompleted" oor:op="fuse"><value>true</value></prop></item>
</oor:items>
`

type session struct {
	exe, tmp, profile string
}

func newSession() (*session, error) {
	exe, err := Find()
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "paperworks-office-")
	if err != nil {
		return nil, err
	}
	s := &session{exe: exe, tmp: tmp, profile: filepath.Join(tmp, "profile")}
	if err := os.MkdirAll(filepath.Join(s.profile, "user"), 0o700); err != nil {
		s.close()
		return nil, err
	}
	return s, os.WriteFile(filepath.Join(s.profile, "user", "registrymodifications.xcu"), []byte(securityXCU), 0o600)
}

func (s *session) close() { os.RemoveAll(s.tmp) }

// commandRunnerKey is an internal per-call process boundary for native tests.
// Production callers always execute the configured LibreOffice process.
type commandRunnerKey struct{}
type commandRunner func(*exec.Cmd) ([]byte, error)

func (s *session) run(ctx context.Context, extraEnv []string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	full := append([]string{"-env:UserInstallation=" + FileURL(s.profile), "--headless", "--norestore", "--nolockcheck", "--nodefault", "--nofirststartwizard"}, args...)
	cmd := exec.CommandContext(ctx, s.exe, full...)
	cmd.Env = append(isolatedEnv(s.tmp), extraEnv...)
	cmd.WaitDelay = 10 * time.Second
	runner, _ := ctx.Value(commandRunnerKey{}).(commandRunner)
	if runner == nil {
		runner = func(c *exec.Cmd) ([]byte, error) { return c.CombinedOutput() }
	}
	out, err := runner(cmd)
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("LibreOffice timed out after %s", runTimeout)
	}
	if err != nil {
		return fmt.Errorf("LibreOffice failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// installMacro writes the Basic library files into the throwaway profile before
// LibreOffice starts. Documents themselves are opened with NEVER_EXECUTE.
func (s *session) installMacro() error {
	basic := filepath.Join(s.profile, "user", "basic")
	dir := filepath.Join(basic, "Standard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	libraryIndex := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE library:library PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "library.dtd">
<library:library xmlns:library="http://openoffice.org/2000/library" library:name="Standard" library:readonly="false" library:passwordprotected="false">
 <library:element library:name="Module1"/>
</library:library>
`
	libraryList := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE library:libraries PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "libraries.dtd">
<library:libraries xmlns:library="http://openoffice.org/2000/library" xmlns:xlink="http://www.w3.org/1999/xlink">
 <library:library library:name="Standard" library:link="false"/>
</library:libraries>
`
	xba := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE script:module PUBLIC "-//OpenOffice.org//DTD OfficeDocument 1.0//EN" "module.dtd">
<script:module xmlns:script="http://openoffice.org/2000/script" script:name="Module1" script:language="StarBasic">` +
		html.EscapeString(refreshMacro) + "</script:module>\n"
	for _, file := range []struct{ path, data string }{
		{filepath.Join(basic, "script.xlc"), libraryList},
		{filepath.Join(dir, "script.xlb"), libraryIndex},
		{filepath.Join(dir, "Module1.xba"), xba},
	} {
		if err := os.WriteFile(file.path, []byte(file.data), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Convert exports src to the given LibreOffice target extension (pdf, docx,
// xlsx, pptx) inside outDir using a plain --convert-to run.
func Convert(ctx context.Context, src, outDir, target string) (string, error) {
	s, err := newSession()
	if err != nil {
		return "", err
	}
	defer s.close()
	if err := s.run(ctx, nil, "--convert-to", target, "--outdir", outDir, src); err != nil {
		return "", err
	}
	out := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))+"."+target)
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return "", errors.New("office conversion produced no output")
	}
	return out, nil
}

const (
	maxRefreshXMLPart = 16 << 20
	maxRefreshPackage = 32 << 20
)

// xlsxHasRefreshWork reports whether an XLSX contains content whose calculation
// or cached rendering can change during a LibreOffice refresh. If inspection is
// uncertain, it returns true so the normal LibreOffice path is used.
func xlsxHasRefreshWork(src string) bool {
	info, err := os.Stat(src)
	if err != nil || info.Size() > maxRefreshPackage {
		return true
	}
	archive, err := zip.OpenReader(src)
	if err != nil {
		return true
	}
	defer archive.Close()
	parts := make(map[string]*zip.File, len(archive.File))
	for _, part := range archive.File {
		if parts[part.Name] != nil {
			return true
		}
		parts[part.Name] = part
		name := "/" + strings.ToLower(part.Name)
		for _, marker := range []string{"/tables/", "/pivot", "/externallinks/", "/connections", "/querytables/", "/calcchain"} {
			if strings.Contains(name, marker) {
				return true
			}
		}
	}
	readXML := func(name string) ([]byte, bool) {
		part := parts[name]
		if part == nil || part.UncompressedSize64 > maxRefreshXMLPart {
			return nil, false
		}
		r, err := part.Open()
		if err != nil {
			return nil, false
		}
		data, readErr := io.ReadAll(io.LimitReader(r, maxRefreshXMLPart+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil || len(data) > maxRefreshXMLPart {
			return nil, false
		}
		return data, true
	}
	containsElement := func(data []byte, tag string) (bool, bool) {
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return false, true
			}
			if err != nil {
				return false, false
			}
			if element, ok := token.(xml.StartElement); ok && element.Name.Local == tag {
				return true, true
			}
		}
	}
	workbook, ok := readXML("xl/workbook.xml")
	if !ok {
		return true
	}
	if hasNames, valid := containsElement(workbook, "definedName"); !valid || hasNames {
		return true
	}
	relationshipsXML, ok := readXML("xl/_rels/workbook.xml.rels")
	if !ok {
		return true
	}
	var relationships struct {
		Items []struct {
			Type       string `xml:"Type,attr"`
			Target     string `xml:"Target,attr"`
			TargetMode string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relationshipsXML, &relationships); err != nil {
		return true
	}
	sheets := 0
	for _, rel := range relationships.Items {
		if !strings.HasSuffix(rel.Type, "/worksheet") {
			continue
		}
		if rel.TargetMode != "" || rel.Target == "" {
			return true
		}
		sheetName := opc.Resolve("xl/workbook.xml", rel.Target)
		sheet, ok := readXML(sheetName)
		if !ok {
			return true
		}
		sheets++
		hasFormula, valid := containsElement(sheet, "f")
		if !valid || hasFormula {
			return true
		}
	}
	return sheets == 0
}

func copyFileExclusive(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("office refresh: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		os.Remove(dst)
		return err
	}
	if err := output.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

// refreshXLSXWithoutOffice handles conservative no-formula workbooks locally.
// Standard chart caches are rebuilt from saved cell values; unsupported or
// ambiguous package features fall back to LibreOffice.
func refreshXLSXWithoutOffice(src, dst string) (bool, error) {
	if xlsxHasRefreshWork(src) {
		return false, nil
	}
	pkg, err := opc.Open(src)
	if err != nil {
		return false, nil
	}
	charts, err := office.ChartParts(pkg)
	if err != nil {
		return false, nil
	}
	if len(charts) > 0 {
		if err := office.RefreshWorkbookChartCaches(pkg); err != nil {
			return false, nil
		}
		return true, pkg.SaveNew(dst)
	}
	return true, copyFileExclusive(src, dst)
}

// Refresh opens src, recalculates formulas, refreshes fields and indexes and
// stores the result at dst with the filter implied by dst's extension.
func Refresh(ctx context.Context, src, dst string, wholeSheets bool) error {
	if strings.EqualFold(filepath.Ext(src), ".xlsx") && strings.EqualFold(filepath.Ext(dst), ".xlsx") && !wholeSheets {
		if handled, err := refreshXLSXWithoutOffice(src, dst); handled {
			return err
		}
	}
	filter := map[string]string{".xlsx": "Calc MS Excel 2007 XML", ".docx": "Office Open XML Text", ".pptx": "Impress MS PowerPoint 2007 XML"}[strings.ToLower(filepath.Ext(dst))]
	if filter == "" {
		filter = "calc_pdf_Export"
		if ext := strings.ToLower(filepath.Ext(src)); ext == ".docx" {
			filter = "writer_pdf_Export"
		} else if ext == ".pptx" {
			filter = "impress_pdf_Export"
		}
	}
	s, err := newSession()
	if err != nil {
		return err
	}
	defer s.close()
	if err := s.installMacro(); err != nil {
		return err
	}
	stageDir := filepath.Join(s.tmp, "output")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return err
	}
	staged := filepath.Join(stageDir, filepath.Base(dst))
	status := filepath.Join(s.tmp, "status.txt")
	env := []string{"PW_IN=" + FileURL(src), "PW_OUT=" + FileURL(staged), "PW_FILTER=" + filter, "PW_STATUS=" + status}
	if wholeSheets {
		env = append(env, "PW_WHOLE_SHEETS=1")
	}
	if err := s.run(ctx, env, "macro:///Standard.Module1.Main"); err != nil {
		return err
	}
	msg, _ := os.ReadFile(status)
	if st := strings.TrimSpace(string(msg)); st != "ok" {
		if st == "" {
			return errors.New("office refresh did not complete")
		}
		return fmt.Errorf("office refresh: %s", strings.TrimPrefix(st, "error: "))
	}
	if st, err := os.Stat(staged); err != nil || st.Size() == 0 {
		return errors.New("office refresh produced no output")
	}
	return copyFileExclusive(staged, dst)
}
