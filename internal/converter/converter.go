// Package converter wraps xpanelconversiontool.cli.exe and handles generation
// of the XML argument files it requires.
package converter

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

const defaultExePath = `C:\Program Files (x86)\Crestron\XPanelConvert\xpanelconversiontool.cli.exe`

// ToolExePath returns the path to xpanelconversiontool.cli.exe, resolved from
// the "tool_path" viper config key (set via XPC_TOOL_PATH env var) or the
// default Crestron installation path.
func ToolExePath() string {
	if p := viper.GetString("tool_path"); p != "" {
		return p
	}
	return defaultExePath
}

// DefaultReportDir returns a timestamped subdirectory inside the OS temp folder
// for storing conversion reports.
func DefaultReportDir() string {
	ts := time.Now().Format("20060102-150405")
	return filepath.Join(os.TempDir(), "xpc-reports", ts)
}

// Run executes xpanelconversiontool.cli.exe with the given arguments.
// In verbose mode stdout and stderr are streamed live to the terminal.
// In quiet mode both are captured; on failure the captured output is written
// to stderr so error details remain visible while successful runs stay silent.
func Run(verbose bool, args ...string) error {
	cmd := exec.Command(ToolExePath(), args...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		if buf.Len() > 0 {
			_, _ = fmt.Fprint(os.Stderr, buf.String())
		}
		return err
	}
	return nil
}

// =============================================================================
// XML argument files
// =============================================================================

// HTMLConversionArgs holds parameters for generating the HTML args XML file
// required by xpanelconversiontool.cli.exe.
type HTMLConversionArgs struct {
	// PassthroughFile, if non-empty, is returned as-is without generating a new
	// file (honours the --html-args flag).
	PassthroughFile      string
	IsFusionServer       bool
	ControlHtmlPath      string
	MacInstallerLink     string
	WindowsInstallerLink string
}

// CNXConnectionArgs holds parameters for generating the CNX args XML file
// required by xpanelconversiontool.cli.exe.
type CNXConnectionArgs struct {
	// PassthroughFile, if non-empty, is returned as-is without generating a new
	// file (honours the --cnx-args flag).
	PassthroughFile string
	Host            string
	Port            int
	IpId            string
	EnableSSL       bool
}

// ResolveHTMLArgs returns the path to an HTML args XML file.
// If args.PassthroughFile is set it is returned directly; otherwise a temp
// file is generated from the other fields and a cleanup function is returned.
func ResolveHTMLArgs(args HTMLConversionArgs) (path string, cleanup func(), err error) {
	if args.PassthroughFile != "" {
		return args.PassthroughFile, nil, nil
	}
	ha := htmlArgsXML{
		IsFusionServer:       args.IsFusionServer,
		ControlHtmlPath:      args.ControlHtmlPath,
		MacInstallerLink:     args.MacInstallerLink,
		WindowsInstallerLink: args.WindowsInstallerLink,
	}
	return writeTempXML("xpc-html-args-*.xml", ha)
}

// ResolveCNXArgs returns the path to a CNX args XML file, or an empty path if
// no CNX host is configured (the --cnx= tool flag is optional).
// If args.PassthroughFile is set it is returned directly.
func ResolveCNXArgs(args CNXConnectionArgs) (path string, cleanup func(), err error) {
	if args.PassthroughFile != "" {
		return args.PassthroughFile, nil, nil
	}
	if args.Host == "" {
		return "", nil, nil
	}
	ca := cnxArgsXML{
		Host:      args.Host,
		Port:      args.Port,
		EnableSSL: args.EnableSSL,
		IpId:      args.IpId,
	}
	return writeTempXML("xpc-cnx-args-*.xml", ca)
}

// --- XML types (unexported) --------------------------------------------------

type htmlArgsXML struct {
	XMLName              xml.Name `xml:"HtmlArgs"`
	IsFusionServer       bool     `xml:"IsFusionServer"`
	ControlHtmlPath      string   `xml:"ControlHtmlPath"`
	MacInstallerLink     string   `xml:"MacInstallerLink"`
	WindowsInstallerLink string   `xml:"WindowsInstallerLink"`
}

type cnxArgsXML struct {
	XMLName   xml.Name `xml:"ConvertCnxArgs"`
	Host      string   `xml:"Host"`
	Port      int      `xml:"Port"`
	EnableSSL bool     `xml:"EnableSSL"`
	IpId      string   `xml:"IpId"`
}

// writeTempXML marshals v as indented XML into a temp file matching pattern.
// The returned cleanup function removes the file.
func writeTempXML(pattern string, v any) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.Remove(f.Name()) }

	enc := xml.NewEncoder(f)
	enc.Indent("", "  ")
	if encErr := enc.Encode(v); encErr != nil {
		f.Close()
		cleanup()
		return "", nil, encErr
	}
	if closeErr := f.Close(); closeErr != nil {
		cleanup()
		return "", nil, closeErr
	}
	return filepath.Clean(f.Name()), cleanup, nil
}
