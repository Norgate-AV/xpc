package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbsLocalPaths_RelativeSourceNoOutput verifies that a relative source path
// is expanded to an absolute path and the default output dir is derived from it.
func TestAbsLocalPaths_RelativeSourceNoOutput(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	rel := filepath.Join("some", "relative", "path")
	absSource, absDest, err := absLocalPaths(rel, "")

	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(absSource), "source should be absolute")
	assert.True(t, filepath.IsAbs(absDest), "output dir should be absolute")
	assert.Equal(t, filepath.Join(cwd, rel), absSource)
	assert.Equal(t, filepath.Join(cwd, rel), absDest)
}

// TestAbsLocalPaths_DotRelativeSource verifies the exact pattern that caused the
// "Invalid URI" error from the Crestron tool (.\path\to\project).
func TestAbsLocalPaths_DotRelativeSource(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	rel := filepath.Join(".", "test", "integration", "Xpanel2_Project.c3prj")
	absSource, absDest, err := absLocalPaths(rel, "")

	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(absSource), "source should be absolute, got: %s", absSource)
	assert.True(t, filepath.IsAbs(absDest), "output dir should be absolute, got: %s", absDest)
	assert.Equal(t, filepath.Join(cwd, "test", "integration", "Xpanel2_Project.c3prj"), absSource)
	assert.NotContains(t, absSource, "."+string(filepath.Separator), "resolved path should contain no relative segments")
}

// TestAbsLocalPaths_RelativeSourceAndOutput verifies both source and output are
// resolved when both are relative.
func TestAbsLocalPaths_RelativeSourceAndOutput(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	rel := filepath.Join("my", "project")
	out := filepath.Join("my", "output")
	absSource, absDest, err := absLocalPaths(rel, out)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, rel), absSource)
	assert.Equal(t, filepath.Join(cwd, out), absDest)
	assert.True(t, filepath.IsAbs(absSource))
	assert.True(t, filepath.IsAbs(absDest))
}

// TestAbsLocalPaths_AbsoluteSourcePreserved verifies that an already-absolute
// source path passes through unchanged.
func TestAbsLocalPaths_AbsoluteSourcePreserved(t *testing.T) {
	t.Parallel()

	// Use a platform-agnostic absolute path via TempDir.
	tmp := t.TempDir()

	absSource, absDest, err := absLocalPaths(tmp, "")

	require.NoError(t, err)
	assert.Equal(t, tmp, absSource)
	assert.Equal(t, tmp, absDest)
}

// TestAbsLocalPaths_ExplicitAbsOutput verifies that an explicit absolute --out
// value is passed through unchanged.
func TestAbsLocalPaths_ExplicitAbsOutput(t *testing.T) {
	t.Parallel()

	srcTmp := t.TempDir()
	outTmp := t.TempDir()

	absSource, absDest, err := absLocalPaths(srcTmp, outTmp)

	require.NoError(t, err)
	assert.Equal(t, srcTmp, absSource)
	assert.Equal(t, outTmp, absDest)
}

// TestAbsLocalPaths_DefaultOutputSameAsSource verifies that omitting --out
// produces an in-place conversion (dest == source). runXPC transparently uses
// a working copy so the original project files are never touched by the tool.
func TestAbsLocalPaths_DefaultOutputSameAsSource(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	absSource, absDest, err := absLocalPaths(tmp, "")

	require.NoError(t, err)
	assert.Equal(t, absSource, absDest,
		"default output should be the source dir (in-place conversion)")
}

// TestAbsLocalPaths_ExplicitDestSameAsSource_Allowed verifies that passing
// --out equal to --dir is accepted. runXPC creates a working copy to protect
// the source files.
func TestAbsLocalPaths_ExplicitDestSameAsSource_Allowed(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	absSource, absDest, err := absLocalPaths(tmp, tmp)

	require.NoError(t, err)
	assert.Equal(t, absSource, absDest)
}

// TestAbsLocalPaths_ExplicitDestSameAsSourceRelative verifies the same-path
// case via different relative forms is also accepted.
func TestAbsLocalPaths_ExplicitDestSameAsSourceRelative(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	_, _, err := absLocalPaths(tmp, tmp+string(filepath.Separator)+".")

	require.NoError(t, err, "same path via different relative form should be accepted")
}

// =============================================================================
// copyToTemp / copyDir / copyFile
// =============================================================================

// TestCopyFile_ContentPreserved verifies byte-level fidelity.
func TestCopyFile_ContentPreserved(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello xpc"), 0o644))

	require.NoError(t, copyFile(src, dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello xpc", string(got))
}

// TestCopyDir_RecursiveCopy verifies nested files and directories are copied.
func TestCopyDir_RecursiveCopy(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	files := map[string]string{
		"a.txt":         "file a",
		"sub/b.txt":     "file b",
		"sub/sub/c.txt": "file c",
	}
	for rel, content := range files {
		full := filepath.Join(srcDir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	dstDir := t.TempDir()
	require.NoError(t, copyDir(srcDir, dstDir))

	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dstDir, rel))
		require.NoError(t, err, "copied file %s should exist", rel)
		assert.Equal(t, want, string(got), "content of %s should match", rel)
	}
}

// TestCopyToTemp_CreatesTempDir verifies a temp directory is created with the
// source contents and that the original is unmodified.
func TestCopyToTemp_CreatesTempDir(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "project.xml"), []byte("data"), 0o644))

	tmpDir, err := copyToTemp(srcDir)
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Temp dir must contain the copied file.
	got, err := os.ReadFile(filepath.Join(tmpDir, "project.xml"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))

	// Original must be untouched.
	orig, err := os.ReadFile(filepath.Join(srcDir, "project.xml"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(orig))
}

// =============================================================================
// removeEmptyDirs
// =============================================================================

// TestRemoveEmptyDirs_RemovesEmptySubdirs verifies that deeply nested empty
// directories are removed bottom-up, including parent directories that become
// empty after their children are removed.
func TestRemoveEmptyDirs_RemovesEmptySubdirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// controls/ → empty leaf (should be removed)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "controls"), 0o755))

	// themes/Int_CT_Neo/ → empty nested dirs (both should be removed)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "themes", "Int_CT_Neo"), 0o755))

	// images/ has a file — must not be removed
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "images"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "images", "logo.png"), []byte("x"), 0o644))

	require.NoError(t, removeEmptyDirs(dir))

	_, err := os.Stat(filepath.Join(dir, "controls"))
	assert.True(t, os.IsNotExist(err), "empty controls/ should be removed")

	_, err = os.Stat(filepath.Join(dir, "themes", "Int_CT_Neo"))
	assert.True(t, os.IsNotExist(err), "empty themes/Int_CT_Neo/ should be removed")

	_, err = os.Stat(filepath.Join(dir, "themes"))
	assert.True(t, os.IsNotExist(err), "themes/ should be removed after its child was removed")

	_, err = os.Stat(filepath.Join(dir, "images"))
	assert.NoError(t, err, "images/ with a file should remain")

	_, err = os.Stat(dir)
	assert.NoError(t, err, "root dir should never be removed")
}

// TestRemoveEmptyDirs_RootPreserved verifies the root directory is never
// removed even when it is completely empty.
func TestRemoveEmptyDirs_RootPreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, removeEmptyDirs(dir))

	_, err := os.Stat(dir)
	assert.NoError(t, err, "root dir must not be removed")
}

// TestRemoveEmptyDirs_NonEmptyDirUntouched verifies that a directory containing
// files is left completely intact.
func TestRemoveEmptyDirs_NonEmptyDirUntouched(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("data"), 0o644))

	require.NoError(t, removeEmptyDirs(dir))

	got, err := os.ReadFile(filepath.Join(dir, "sub", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(got), "file inside non-empty dir should be untouched")
}
