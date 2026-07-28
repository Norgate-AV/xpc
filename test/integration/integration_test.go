//go:build integration
// +build integration

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultToolPath = `C:\Program Files (x86)\Crestron\XPanelConvert\xpanelconversiontool.cli.exe`

// xpcBinary returns the path to the built xpc binary, skipping the test if it
// has not been built yet.
func xpcBinary(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine test file path")

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	name := "xpc.exe"
	if runtime.GOOS != "windows" {
		name = "xpc"
	}

	binPath := filepath.Join(projectRoot, "bin", name)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Skipf("xpc binary not found at %s; run 'make build' first", binPath)
	}

	return binPath
}

// runXPC executes the xpc binary with the given arguments and returns stdout,
// stderr, and the exit code.
func runXPC(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runXPCWithEnv(t, nil, args...)
}

// runXPCWithEnv executes the xpc binary with the given arguments and extra
// environment variables (in "KEY=VALUE" form), returning stdout, stderr, and
// the exit code.
func runXPCWithEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	bin := xpcBinary(t)
	cmd := exec.Command(bin, args...)

	// Inherit the current environment, then append any overrides.
	cmd.Env = append(os.Environ(), env...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// conversionToolAvailable returns true if xpanelconversiontool.cli.exe can be
// found, either via XPC_TOOL_PATH or the default installation path.
func conversionToolAvailable() bool {
	toolPath := os.Getenv("XPC_TOOL_PATH")
	if toolPath == "" {
		toolPath = defaultToolPath
	}

	_, err := os.Stat(toolPath)
	return err == nil
}

// --- Help / version ----------------------------------------------------------

// TestIntegration_Help verifies that --help exits cleanly and produces output.
func TestIntegration_Help(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runXPC(t, "--help")

	assert.Equal(t, 0, exitCode, "--help should exit with code 0")
	assert.Contains(t, stdout, "xpc", "help output should mention the tool name")
	assert.Contains(t, stdout, "--host", "help output should document --host flag")
	assert.Contains(t, stdout, "--dir", "help output should document --dir flag")
}

// --- Flag validation (no external tool required) ----------------------------

// TestIntegration_NoFlags_RequiresMode verifies that running with no flags fails
// with a clear message indicating one of the mode flags is required.
func TestIntegration_NoFlags_RequiresMode(t *testing.T) {
	t.Parallel()

	_, stderr, exitCode := runXPC(t)

	assert.NotEqual(t, 0, exitCode, "expected non-zero exit code with no flags")
	assert.Contains(t, stderr, "--host", "error should reference --host flag")
}

// TestIntegration_HostAndFTPMutuallyExclusive verifies that --host and --ftp
// cannot be combined.
func TestIntegration_HostAndFTPMutuallyExclusive(t *testing.T) {
	t.Parallel()

	_, stderr, exitCode := runXPC(t,
		"--host", "192.168.1.1",
		"--ftp", "ftp://user@192.168.1.2/html",
	)

	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "mutually exclusive", "error should mention mutual exclusion")
}

// TestIntegration_HostAndDirMutuallyExclusive verifies that --host and --dir
// cannot be combined.
func TestIntegration_HostAndDirMutuallyExclusive(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, stderr, exitCode := runXPC(t, "--host", "192.168.1.1", "--dir", tmpDir)

	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "--host", "error should reference the conflicting flag")
}

// TestIntegration_FTPAndDirMutuallyExclusive verifies that --ftp and --dir
// cannot be combined.
func TestIntegration_FTPAndDirMutuallyExclusive(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, stderr, exitCode := runXPC(t,
		"--ftp", "ftp://user@192.168.1.1/html",
		"--dir", tmpDir,
	)

	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "--ftp", "error should reference the conflicting flag")
}

// --- Local mode (tool invocation required) -----------------------------------

// TestIntegration_LocalMode_EmptyDir runs a local conversion against an empty
// temp directory. This test requires xpanelconversiontool.cli.exe to be present.
func TestIntegration_LocalMode_EmptyDir(t *testing.T) {
	if !conversionToolAvailable() {
		t.Skip("xpanelconversiontool.cli.exe not available; skipping conversion test")
	}

	srcDir := t.TempDir()
	outDir := t.TempDir()

	_, stderr, _ := runXPC(t, "--dir", srcDir, "--out", outDir, "--force", "--verbose")

	// The tool will be invoked; we just verify xpc progressed to the convert step
	assert.Contains(t, stderr, "Converting", "should reach the conversion step")
}

// TestIntegration_LocalMode_OutputDirCreated verifies that the source directory
// is the output location for in-place conversion (the default when --out is
// omitted). The directory must still exist after the run.
func TestIntegration_LocalMode_OutputDirCreated(t *testing.T) {
	if !conversionToolAvailable() {
		t.Skip("xpanelconversiontool.cli.exe not available; skipping conversion test")
	}

	srcDir := t.TempDir()

	runXPC(t, "--dir", srcDir)

	_, err := os.Stat(srcDir)
	assert.NoError(t, err, "source dir should still exist after in-place conversion")
}

// TestIntegration_LocalMode_ToolNotFound verifies that xpc exits with an error
// and a descriptive message when the conversion tool cannot be found.
func TestIntegration_LocalMode_ToolNotFound(t *testing.T) {
	if conversionToolAvailable() {
		t.Skip("xpanelconversiontool.cli.exe is present; skipping tool-not-found test")
	}

	srcDir := t.TempDir()
	outDir := t.TempDir()

	_, stderr, exitCode := runXPCWithEnv(t,
		[]string{"XPC_TOOL_PATH=/nonexistent/path/to/tool.exe"},
		"--dir", srcDir,
		"--out", outDir,
		"--force",
	)

	assert.NotEqual(t, 0, exitCode, "should fail when tool is not found")
	assert.NotEmpty(t, stderr, "should produce error output")
}

// --- Path resolution ---------------------------------------------------------

// TestIntegration_RelativeDirNoInvalidURIError verifies the regression fix:
// passing a relative --dir must NOT produce the Crestron tool's "Invalid URI"
// error.  Before the fix, relative paths were forwarded verbatim and the tool
// rejected them with "Invalid URI: The format of the URI could not be
// determined."
func TestIntegration_RelativeDirNoInvalidURIError(t *testing.T) {
	t.Parallel()

	// Use a relative path (dot-prefixed, matching the exact pattern that
	// triggered the original bug).
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine test file path")

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	// Make the path relative to the project root by changing the working
	// directory of the subprocess.
	bin := xpcBinary(t)
	cmd := exec.Command(bin, "--dir", filepath.Join(".", "test", "integration"))
	cmd.Dir = projectRoot

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	_ = cmd.Run()

	assert.NotContains(t, errBuf.String(), "Invalid URI",
		"relative path should be resolved to absolute before reaching the tool")
}

// TestIntegration_RelativeDirOutputIsAbsolute verifies that when a relative
// --dir is given the conversion still proceeds (no "Invalid URI" error from
// the tool about non-absolute paths).
func TestIntegration_RelativeDirOutputIsAbsolute(t *testing.T) {
	if !conversionToolAvailable() {
		t.Skip("xpanelconversiontool.cli.exe not available; skipping output-path test")
	}

	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine test file path")

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	srcDir := t.TempDir()
	relSrc, err := filepath.Rel(projectRoot, srcDir)
	require.NoError(t, err, "should be able to make temp dir relative to project root")

	bin := xpcBinary(t)
	cmd := exec.Command(bin, "--dir", relSrc)
	cmd.Dir = projectRoot

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	_ = cmd.Run()

	assert.NotContains(t, errBuf.String(), "Invalid URI",
		"relative path should be resolved to absolute before reaching the tool")
}

// --- Source/output path guard ------------------------------------------------

// TestIntegration_InPlace_SourceFilesPreserved is the critical regression test
// for the in-place destructive bug. xpc must create a working copy before
// invoking the Crestron tool so the original source files are never deleted,
// regardless of whether the conversion succeeds or fails.
func TestIntegration_InPlace_SourceFilesPreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	sentinels := []string{"file1.txt", "file2.txt", "sub/file3.txt"}
	for _, name := range sentinels {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte("sentinel"), 0o644))
	}

	// Run in-place (no --out). Whether the Crestron tool is present or not,
	// xpc must not delete the original files.
	runXPC(t, "--dir", dir)

	for _, name := range sentinels {
		full := filepath.Join(dir, name)
		_, err := os.Stat(full)
		assert.NoError(t, err, "sentinel file %s must survive in-place conversion", name)
	}
}

// TestIntegration_HostNoCredentials_NonInteractiveError verifies that --host
// without any password env vars fails immediately in a non-interactive
// environment with a clear message rather than blocking for terminal input.
func TestIntegration_HostNoCredentials_NonInteractiveError(t *testing.T) {
	t.Parallel()

	_, stderr, exitCode := runXPCWithEnv(t,
		[]string{
			"XPC_USER=testuser",
			"XPC_PASSWORD=",
			"XPC_192_168_255_255_PASSWORD=",
		},
		"--host", "192.168.255.255",
	)

	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "stdin is not a terminal",
		"non-interactive run without password should report terminal error, not hang")
}

// TestIntegration_FTPInvalidScheme verifies that a non-ftp:// URL is rejected
// with a clear error referencing the expected scheme.
func TestIntegration_FTPInvalidScheme(t *testing.T) {
	t.Parallel()

	_, stderr, exitCode := runXPC(t, "--ftp", "http://user@192.168.1.1/html")

	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "ftp://", "error should hint at the required scheme")
}

// TestIntegration_FTPPasswordFromEnv verifies that XPC_FTP_PASSWORD is used to
// resolve credentials without prompting.  xpc is expected to fail at the
// network layer (connection refused), not at credential resolution.
func TestIntegration_FTPPasswordFromEnv(t *testing.T) {
	t.Parallel()

	// Port 1 on localhost is virtually never open; connection is refused fast.
	_, stderr, exitCode := runXPCWithEnv(t,
		[]string{
			"XPC_USER=testuser",
			"XPC_FTP_PASSWORD=testpass",
		},
		"--ftp", "ftp://127.0.0.1:1/html",
	)

	assert.NotEqual(t, 0, exitCode)
	assert.NotContains(t, stderr, "stdin is not a terminal",
		"password resolved from env should not trigger interactive prompt error")
}

// --- Source/output path guard ------------------------------------------------

// TestIntegration_ExplicitOutSameAsDir_Rejected verifies that passing an
// explicit --out equal to --dir is rejected with a clear message directing the
// user to omit --out instead (omitting --out triggers the in-place default).
func TestIntegration_ExplicitOutSameAsDir_Rejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, stderr, exitCode := runXPC(t, "--dir", dir, "--out", dir)

	assert.NotEqual(t, 0, exitCode, "should fail when --out explicitly equals --dir")
	assert.Contains(t, stderr, "--out resolves to the same path as --dir",
		"error should guide the user to omit --out for in-place conversion")
}

// TestIntegration_DefaultInPlace_Allowed verifies that running without --out
// is accepted (in-place conversion is the default and is always allowed).
func TestIntegration_DefaultInPlace_Allowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Without --out, xpc uses in-place mode.  Without the Crestron tool the
	// binary will fail at the conversion step, but it must NOT fail at the
	// path-validation step.
	_, stderr, exitCode := runXPC(t, "--dir", dir)

	// Only acceptable failure reason is the conversion tool not being found or
	// failing — never a path validation error.
	if exitCode != 0 {
		assert.NotContains(t, stderr, "same path",
			"path validation should not reject in-place default")
		assert.NotContains(t, stderr, "--out resolves to the same path",
			"path validation should not reject in-place default")
	}
}
