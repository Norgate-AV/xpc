package transfer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Config.RemotePath
// =============================================================================

func TestRemotePath_WindowsBackslashBothEnds(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: `\html\`}
	assert.Equal(t, "/html", c.RemotePath())
}

func TestRemotePath_UnixForwardSlashBothEnds(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: "/html/"}
	assert.Equal(t, "/html", c.RemotePath())
}

func TestRemotePath_AlreadyNormalized(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: "/html"}
	assert.Equal(t, "/html", c.RemotePath())
}

func TestRemotePath_NoLeadingSlash(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: "html"}
	assert.Equal(t, "/html", c.RemotePath())
}

func TestRemotePath_NestedWindowsPath(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: `\WebXPanel\src\`}
	assert.Equal(t, "/WebXPanel/src", c.RemotePath())
}

func TestRemotePath_NestedUnixPath(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: "/var/www/html/"}
	assert.Equal(t, "/var/www/html", c.RemotePath())
}

func TestRemotePath_MixedSlashes(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: `\mixed/path\`}
	assert.Equal(t, "/mixed/path", c.RemotePath())
}

// TestRemotePath_DefaultFlagValue verifies the default --ctrl-path value
// (\html\) normalises correctly.
func TestRemotePath_DefaultFlagValue(t *testing.T) {
	t.Parallel()
	c := Config{CtrlPath: `\html\`}
	assert.Equal(t, "/html", c.RemotePath())
}
