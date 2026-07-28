package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ToolExePath
// =============================================================================

func TestToolExePath_ReturnsDefaultWhenNotConfigured(t *testing.T) {
	orig := viper.GetString("tool_path")
	viper.Set("tool_path", "")
	t.Cleanup(func() { viper.Set("tool_path", orig) })

	assert.Equal(t, defaultExePath, ToolExePath())
}

func TestToolExePath_ViperOverride(t *testing.T) {
	viper.Set("tool_path", `C:\Custom\tool.exe`)
	t.Cleanup(func() { viper.Set("tool_path", "") })

	assert.Equal(t, `C:\Custom\tool.exe`, ToolExePath())
}

// =============================================================================
// DefaultReportDir
// =============================================================================

func TestDefaultReportDir_IsAbsolute(t *testing.T) {
	t.Parallel()
	assert.True(t, filepath.IsAbs(DefaultReportDir()))
}

func TestDefaultReportDir_UnderTempDir(t *testing.T) {
	t.Parallel()
	dir := DefaultReportDir()
	assert.True(t, strings.HasPrefix(dir, os.TempDir()),
		"report dir should be under %s, got: %s", os.TempDir(), dir)
}

func TestDefaultReportDir_ContainsXPCReports(t *testing.T) {
	t.Parallel()
	assert.Contains(t, DefaultReportDir(), "xpc-reports")
}

// =============================================================================
// writeTempXML (via public API)
// =============================================================================

func TestWriteTempXML_FileIsCreated(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveHTMLArgs(HTMLConversionArgs{})
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "temp file should exist")
}

func TestWriteTempXML_CleanupRemovesFile(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveHTMLArgs(HTMLConversionArgs{})
	require.NoError(t, err)

	cleanup()

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "temp file should be removed after cleanup")
}

func TestWriteTempXML_PathIsAbsolute(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveHTMLArgs(HTMLConversionArgs{})
	require.NoError(t, err)
	defer cleanup()

	assert.True(t, filepath.IsAbs(path), "returned path should be absolute, got: %s", path)
}

// =============================================================================
// htmlArgsXML serialization (internal type, accessed within package)
// =============================================================================

func TestHTMLArgsXML_DefaultValues(t *testing.T) {
	t.Parallel()

	path, cleanup, err := writeTempXML("xpc-html-args-*.xml", htmlArgsXML{})
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	xmlStr := string(data)

	assert.Contains(t, xmlStr, "<HtmlArgs>")
	assert.Contains(t, xmlStr, "<IsFusionServer>false</IsFusionServer>")
}

func TestHTMLArgsXML_FusionServerTrue(t *testing.T) {
	t.Parallel()

	path, cleanup, err := writeTempXML("xpc-html-args-*.xml", htmlArgsXML{IsFusionServer: true})
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<IsFusionServer>true</IsFusionServer>")
}

func TestHTMLArgsXML_InstallerLinks(t *testing.T) {
	t.Parallel()

	ha := htmlArgsXML{
		MacInstallerLink:     "https://example.com/mac.pkg",
		WindowsInstallerLink: "https://example.com/win.exe",
	}
	path, cleanup, err := writeTempXML("xpc-html-args-*.xml", ha)
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	xmlStr := string(data)

	assert.Contains(t, xmlStr, "<MacInstallerLink>https://example.com/mac.pkg</MacInstallerLink>")
	assert.Contains(t, xmlStr, "<WindowsInstallerLink>https://example.com/win.exe</WindowsInstallerLink>")
}

// =============================================================================
// cnxArgsXML serialization
// =============================================================================

func TestCNXArgsXML_AllFields(t *testing.T) {
	t.Parallel()

	ca := cnxArgsXML{Host: "192.168.1.100", Port: 41794, EnableSSL: false, IpId: "03"}
	path, cleanup, err := writeTempXML("xpc-cnx-args-*.xml", ca)
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	xmlStr := string(data)

	assert.Contains(t, xmlStr, "<ConvertCnxArgs>")
	assert.Contains(t, xmlStr, "<Host>192.168.1.100</Host>")
	assert.Contains(t, xmlStr, "<Port>41794</Port>")
	assert.Contains(t, xmlStr, "<EnableSSL>false</EnableSSL>")
	assert.Contains(t, xmlStr, "<IpId>03</IpId>")
}

func TestCNXArgsXML_SSLEnabled(t *testing.T) {
	t.Parallel()

	ca := cnxArgsXML{Host: "host", Port: 41794, EnableSSL: true, IpId: "0A"}
	path, cleanup, err := writeTempXML("xpc-cnx-args-*.xml", ca)
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Contains(t, string(data), "<EnableSSL>true</EnableSSL>")
	assert.Contains(t, string(data), "<IpId>0A</IpId>")
}

// =============================================================================
// ResolveHTMLArgs
// =============================================================================

func TestResolveHTMLArgs_PassThrough(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp("", "html-args-*.xml")
	require.NoError(t, err)
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	path, cleanup, err := ResolveHTMLArgs(HTMLConversionArgs{PassthroughFile: tmpFile.Name()})

	require.NoError(t, err)
	assert.Nil(t, cleanup, "pass-through should not register a cleanup")
	assert.Equal(t, tmpFile.Name(), path)
}

func TestResolveHTMLArgs_GeneratesTempFile(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveHTMLArgs(HTMLConversionArgs{})
	require.NoError(t, err)
	require.NotNil(t, cleanup, "should return a cleanup function")
	defer cleanup()

	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "generated file should exist")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<HtmlArgs>")
}

func TestResolveHTMLArgs_FusionAndCtrlPathEmbedded(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveHTMLArgs(HTMLConversionArgs{
		IsFusionServer:  true,
		ControlHtmlPath: `\WebXpanel.c3prj\`,
	})
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	xmlStr := string(data)

	assert.Contains(t, xmlStr, "<IsFusionServer>true</IsFusionServer>")
	assert.Contains(t, xmlStr, `<ControlHtmlPath>\WebXpanel.c3prj\</ControlHtmlPath>`)
}

// =============================================================================
// ResolveCNXArgs
// =============================================================================

func TestResolveCNXArgs_EmptyHostReturnsEmpty(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveCNXArgs(CNXConnectionArgs{})

	require.NoError(t, err)
	assert.Empty(t, path)
	assert.Nil(t, cleanup)
}

func TestResolveCNXArgs_PassThrough(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp("", "cnx-args-*.xml")
	require.NoError(t, err)
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	path, cleanup, err := ResolveCNXArgs(CNXConnectionArgs{
		PassthroughFile: tmpFile.Name(),
		Host:            "192.168.1.100",
	})

	require.NoError(t, err)
	assert.Nil(t, cleanup)
	assert.Equal(t, tmpFile.Name(), path)
}

func TestResolveCNXArgs_GeneratesTempFile(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveCNXArgs(CNXConnectionArgs{
		Host: "192.168.1.100",
		Port: 41794,
		IpId: "03",
	})
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	xmlStr := string(data)

	assert.Contains(t, xmlStr, "<ConvertCnxArgs>")
	assert.Contains(t, xmlStr, "<Host>192.168.1.100</Host>")
	assert.Contains(t, xmlStr, "<IpId>03</IpId>")
}

func TestResolveCNXArgs_DefaultPort(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveCNXArgs(CNXConnectionArgs{Host: "192.168.1.100", Port: 41794})
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<Port>41794</Port>")
}

func TestResolveCNXArgs_SSLFlag(t *testing.T) {
	t.Parallel()

	path, cleanup, err := ResolveCNXArgs(CNXConnectionArgs{Host: "host", Port: 41794, EnableSSL: true})
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<EnableSSL>true</EnableSSL>")
}
