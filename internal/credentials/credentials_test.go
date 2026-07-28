package credentials

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

// isStdinTerminal reports whether the test process's stdin is a terminal.
// Tests that exercise the promptPassword code-path skip when this returns true
// to avoid blocking for interactive input.
func isStdinTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// =============================================================================
// EnvSafe
// =============================================================================

func TestEnvSafe_IPAddress(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "192_168_1_1", EnvSafe("192.168.1.1"))
}

func TestEnvSafe_ConvertsToUpperCase(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "MYHOST", EnvSafe("myhost"))
}

func TestEnvSafe_AlreadyUpperCase(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "HOSTNAME", EnvSafe("HOSTNAME"))
}

func TestEnvSafe_HyphensAndDots(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "HOST_NAME_01", EnvSafe("host-name.01"))
}

func TestEnvSafe_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", EnvSafe(""))
}

// =============================================================================
// ResolveUser
// =============================================================================

func TestResolveUser_FlagValue(t *testing.T) {
	t.Parallel()
	user, err := ResolveUser("admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", user)
}

func TestResolveUser_FlagTakesPriorityOverEnv(t *testing.T) {
	t.Setenv("XPC_USER", "envuser")
	user, err := ResolveUser("flaguser")
	require.NoError(t, err)
	assert.Equal(t, "flaguser", user)
}

func TestResolveUser_FromXPCUserEnv(t *testing.T) {
	t.Setenv("XPC_USER", "envuser")
	user, err := ResolveUser("")
	require.NoError(t, err)
	assert.Equal(t, "envuser", user)
}

func TestResolveUser_FromViperConfig(t *testing.T) {
	t.Setenv("XPC_USER", "")
	viper.Set("user", "viperuser")
	t.Cleanup(func() { viper.Set("user", "") })

	user, err := ResolveUser("")
	require.NoError(t, err)
	assert.Equal(t, "viperuser", user)
}

func TestResolveUser_NoSource_ReturnsError(t *testing.T) {
	t.Setenv("XPC_USER", "")
	viper.Set("user", "")
	t.Cleanup(func() { viper.Set("user", "") })

	_, err := ResolveUser("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "username required")
}

// =============================================================================
// ResolvePassword
// =============================================================================

func TestResolvePassword_HostSpecificEnv(t *testing.T) {
	t.Setenv("XPC_192_168_1_1_PASSWORD", "hostpass")
	t.Setenv("XPC_PASSWORD", "genericpass")

	pass, err := ResolvePassword("192.168.1.1")
	require.NoError(t, err)
	assert.Equal(t, "hostpass", pass)
}

func TestResolvePassword_GenericEnvFallback(t *testing.T) {
	t.Setenv("XPC_192_168_1_1_PASSWORD", "")
	t.Setenv("XPC_PASSWORD", "genericpass")

	pass, err := ResolvePassword("192.168.1.1")
	require.NoError(t, err)
	assert.Equal(t, "genericpass", pass)
}

func TestResolvePassword_NonInteractive_ReturnsError(t *testing.T) {
	if isStdinTerminal() {
		t.Skip("stdin is a terminal; non-interactive error path covered by integration tests")
	}

	t.Setenv("XPC_192_168_1_1_PASSWORD", "")
	t.Setenv("XPC_PASSWORD", "")

	_, err := ResolvePassword("192.168.1.1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stdin is not a terminal")
}

// =============================================================================
// ResolveFTPURL
// =============================================================================

func TestResolveFTPURL_NonFTPSchemeRejected(t *testing.T) {
	t.Parallel()
	_, err := ResolveFTPURL("http://user@host/path", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ftp://")
}

func TestResolveFTPURL_MalformedURL(t *testing.T) {
	t.Parallel()
	_, err := ResolveFTPURL("://bad url", "")
	assert.Error(t, err)
}

func TestResolveFTPURL_PasswordFromXPCFTPPassword(t *testing.T) {
	t.Setenv("XPC_FTP_PASSWORD", "ftppass")
	t.Setenv("XPC_PASSWORD", "")
	t.Setenv("XPC_USER", "ftpuser")

	result, err := ResolveFTPURL("ftp://192.168.1.1/html", "")
	require.NoError(t, err)
	assert.Contains(t, result, "ftppass")
	assert.Contains(t, result, "ftpuser")
}

func TestResolveFTPURL_PasswordFromXPCPasswordFallback(t *testing.T) {
	t.Setenv("XPC_FTP_PASSWORD", "")
	t.Setenv("XPC_PASSWORD", "genericpass")
	t.Setenv("XPC_USER", "user")

	result, err := ResolveFTPURL("ftp://192.168.1.1/html", "")
	require.NoError(t, err)
	assert.Contains(t, result, "genericpass")
}

func TestResolveFTPURL_UsernameFromFlagUser(t *testing.T) {
	t.Setenv("XPC_FTP_PASSWORD", "pass")
	t.Setenv("XPC_USER", "")

	result, err := ResolveFTPURL("ftp://192.168.1.1/html", "flaguser")
	require.NoError(t, err)
	assert.Contains(t, result, "flaguser")
}

func TestResolveFTPURL_UsernameFromURL(t *testing.T) {
	t.Setenv("XPC_FTP_PASSWORD", "pass")

	result, err := ResolveFTPURL("ftp://urluser@192.168.1.1/html", "")
	require.NoError(t, err)
	assert.Contains(t, result, "urluser")
}

func TestResolveFTPURL_NonInteractive_NoPassword(t *testing.T) {
	if isStdinTerminal() {
		t.Skip("stdin is a terminal; non-interactive error path covered by integration tests")
	}

	t.Setenv("XPC_FTP_PASSWORD", "")
	t.Setenv("XPC_PASSWORD", "")
	t.Setenv("XPC_USER", "user")

	_, err := ResolveFTPURL("ftp://192.168.1.1/html", "user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stdin is not a terminal")
}
