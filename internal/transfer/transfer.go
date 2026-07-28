// Package transfer provides native SFTP and FTP implementations for downloading
// and uploading Crestron XPanel projects to/from remote processors.
//
// These replace xpanelconversiontool.cli.exe for file transfer, removing the
// Crestron Toolbox dependency entirely — the underlying tool is only invoked
// for the convert step.
package transfer

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	jftp "github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Config holds the connection parameters for a get/put operation.
// Exactly one of Host (SFTP) or FtpURL (FTP) must be set.
type Config struct {
	// SFTP (--host)
	Host     string
	Port     int
	User     string
	Pass     string
	CtrlPath string // remote directory; normalised to Unix style in RemotePath()
	Insecure bool   // skip SSH host key verification

	// FTP (--ftp); the URL already contains the path
	FtpURL string

	Verbose bool // mirror the --verbose flag for progress output
}

// Download fetches the remote project directory into localDir.
func (c Config) Download(localDir string) error {
	if c.FtpURL != "" {
		return ftpGet(c.FtpURL, localDir, c.Verbose)
	}
	client, done, err := c.openSFTP()
	if err != nil {
		return err
	}
	defer done()
	return sftpGet(client, c.RemotePath(), localDir, c.Verbose)
}

// Upload pushes localDir to the remote project directory.
func (c Config) Upload(localDir string) error {
	if c.FtpURL != "" {
		return ftpPut(c.FtpURL, localDir, c.Verbose)
	}
	client, done, err := c.openSFTP()
	if err != nil {
		return err
	}
	defer done()
	return sftpPut(client, localDir, c.RemotePath(), c.Verbose)
}

// RemotePath normalises CtrlPath to a Unix-style absolute path for SFTP.
// Both "\html\" and "/html/" resolve to "/html".
func (c Config) RemotePath() string {
	p := strings.ReplaceAll(c.CtrlPath, `\`, `/`)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// openSFTP dials the SSH server and returns an authenticated SFTP client.
// The returned done function must be called to close the connection.
func (c Config) openSFTP() (*sftp.Client, func(), error) {
	cb, err := makeHostKeyCallback(fmt.Sprintf("%s:%d", c.Host, c.Port), c.Insecure)
	if err != nil {
		return nil, nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            c.User,
		Auth:            []ssh.AuthMethod{ssh.Password(c.Pass)},
		HostKeyCallback: cb,
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH connection to %s: %w", addr, err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("SFTP session on %s: %w", addr, err)
	}

	return client, func() { client.Close(); conn.Close() }, nil
}

// =============================================================================
// SFTP helpers
// =============================================================================

func sftpGet(client *sftp.Client, remotePath, localPath string, verbose bool) error {
	entries, err := client.ReadDir(remotePath)
	if err != nil {
		return fmt.Errorf("SFTP list %s: %w", remotePath, err)
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		rp := path.Join(remotePath, e.Name())
		lp := filepath.Join(localPath, e.Name())
		if e.IsDir() {
			if err := sftpGet(client, rp, lp, verbose); err != nil {
				return err
			}
		} else {
			if verbose {
				fmt.Fprintf(os.Stderr, "  ↓ %s\n", rp)
			}
			if err := sftpGetFile(client, rp, lp); err != nil {
				return err
			}
		}
	}
	return nil
}

func sftpPut(client *sftp.Client, localPath, remotePath string, verbose bool) error {
	if err := client.MkdirAll(remotePath); err != nil {
		return fmt.Errorf("SFTP mkdir %s: %w", remotePath, err)
	}
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		lp := filepath.Join(localPath, e.Name())
		rp := path.Join(remotePath, e.Name())
		if e.IsDir() {
			if err := sftpPut(client, lp, rp, verbose); err != nil {
				return err
			}
		} else {
			if verbose {
				fmt.Fprintf(os.Stderr, "  ↑ %s\n", rp)
			}
			if err := sftpPutFile(client, lp, rp); err != nil {
				return err
			}
		}
	}
	return nil
}

func sftpGetFile(client *sftp.Client, remotePath, localPath string) error {
	src, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("SFTP open %s: %w", remotePath, err)
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func sftpPutFile(client *sftp.Client, localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("SFTP create %s: %w", remotePath, err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// =============================================================================
// FTP helpers
// =============================================================================

func ftpGet(resolvedURL, localDir string, verbose bool) error {
	conn, remotePath, err := ftpDial(resolvedURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Quit() }()
	return ftpGetDir(conn, remotePath, localDir, verbose)
}

func ftpPut(resolvedURL, localDir string, verbose bool) error {
	conn, remotePath, err := ftpDial(resolvedURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Quit() }()
	return ftpPutDir(conn, localDir, remotePath, verbose)
}

func ftpDial(resolvedURL string) (*jftp.ServerConn, string, error) {
	u, err := url.Parse(resolvedURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid FTP URL: %w", err)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "21"
	}

	conn, err := jftp.Dial(host+":"+port, jftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, "", fmt.Errorf("FTP connection to %s: %w", host, err)
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	if err := conn.Login(user, pass); err != nil {
		_ = conn.Quit()
		return nil, "", fmt.Errorf("FTP login as %s@%s: %w", user, host, err)
	}

	remotePath := u.Path
	if remotePath == "" {
		remotePath = "/"
	}
	return conn, remotePath, nil
}

func ftpGetDir(conn *jftp.ServerConn, remotePath, localPath string, verbose bool) error {
	entries, err := conn.List(remotePath)
	if err != nil {
		return fmt.Errorf("FTP list %s: %w", remotePath, err)
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		rp := path.Join(remotePath, e.Name)
		lp := filepath.Join(localPath, e.Name)
		switch e.Type {
		case jftp.EntryTypeFolder:
			if err := ftpGetDir(conn, rp, lp, verbose); err != nil {
				return err
			}
		case jftp.EntryTypeFile:
			if verbose {
				fmt.Fprintf(os.Stderr, "  ↓ %s\n", rp)
			}
			if err := ftpGetFile(conn, rp, lp); err != nil {
				return err
			}
		}
	}
	return nil
}

func ftpPutDir(conn *jftp.ServerConn, localPath, remotePath string, verbose bool) error {
	_ = conn.MakeDir(remotePath) // ignore error — directory may already exist
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		lp := filepath.Join(localPath, e.Name())
		rp := path.Join(remotePath, e.Name())
		if e.IsDir() {
			if err := ftpPutDir(conn, lp, rp, verbose); err != nil {
				return err
			}
		} else {
			if verbose {
				fmt.Fprintf(os.Stderr, "  ↑ %s\n", rp)
			}
			if err := ftpPutFile(conn, lp, rp); err != nil {
				return err
			}
		}
	}
	return nil
}

func ftpGetFile(conn *jftp.ServerConn, remotePath, localPath string) error {
	r, err := conn.Retr(remotePath)
	if err != nil {
		return fmt.Errorf("FTP retrieve %s: %w", remotePath, err)
	}
	defer r.Close()

	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}

func ftpPutFile(conn *jftp.ServerConn, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return conn.Stor(remotePath, f)
}

// =============================================================================
// SSH host key verification
// =============================================================================

func makeHostKeyCallback(hostWithPort string, insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		fmt.Fprintln(os.Stderr, "Warning: SSH host key verification is disabled (--insecure).")
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(sshDir, 0o700)
	knownHostsFile := filepath.Join(sshDir, "known_hosts")

	var strictCB ssh.HostKeyCallback
	if _, err := os.Stat(knownHostsFile); err == nil {
		if strictCB, err = knownhosts.New(knownHostsFile); err != nil {
			return nil, fmt.Errorf("parsing known_hosts: %w", err)
		}
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if strictCB != nil {
			err := strictCB(hostname, remote, key)
			if err == nil {
				return nil
			}
			var ke *knownhosts.KeyError
			if !errors.As(err, &ke) {
				return err
			}
			if len(ke.Want) > 0 {
				return fmt.Errorf(
					"⚠  REMOTE HOST KEY HAS CHANGED for %s\n"+
						"   This may indicate a man-in-the-middle attack.\n"+
						"   If the host was reinstalled, remove its old entry from:\n"+
						"   %s",
					hostname, knownHostsFile)
			}
		}
		return promptAcceptHostKey(hostname, key, knownHostsFile)
	}, nil
}

func promptAcceptHostKey(hostname string, key ssh.PublicKey, knownHostsFile string) error {
	fp := ssh.FingerprintSHA256(key)

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf(
			"unknown host key for %s (%s fingerprint: %s)\n"+
				"Run interactively to accept, add it to %s manually,\n"+
				"or use --insecure to bypass verification.",
			hostname, key.Type(), fp, knownHostsFile)
	}

	fmt.Fprintf(os.Stderr,
		"The authenticity of host %q can't be established.\n"+
			"%s key fingerprint is %s\n"+
			"Are you sure you want to continue connecting? (yes/no): ",
		hostname, key.Type(), fp)

	var answer string
	if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	fmt.Fprintln(os.Stderr)

	if strings.ToLower(strings.TrimSpace(answer)) != "yes" {
		return fmt.Errorf("host key not accepted; connection cancelled")
	}

	f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("updating known_hosts: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key)); err != nil {
		return fmt.Errorf("writing to known_hosts: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Warning: permanently added %q (%s) to known hosts.\n", hostname, key.Type())
	return nil
}
