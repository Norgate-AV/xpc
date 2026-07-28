// Package credentials handles secure resolution of usernames and passwords for
// remote connections. Credentials are never accepted via command-line flags —
// only from environment variables or interactive prompts.
package credentials

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/viper"
	"golang.org/x/term"
)

// EnvSafe converts a string to a safe environment variable name segment:
// uppercased with non-alphanumeric characters replaced by '_'.
// e.g. "192.168.1.1" → "192_168_1_1"
func EnvSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToUpper(r)
		}
		return '_'
	}, s)
}

// ResolveUser returns the username from (in priority order):
// the provided flag value, XPC_USER env var, or the viper "user" config key.
func ResolveUser(flagUser string) (string, error) {
	if flagUser != "" {
		return flagUser, nil
	}
	if u := os.Getenv("XPC_USER"); u != "" {
		return u, nil
	}
	if u := viper.GetString("user"); u != "" {
		return u, nil
	}
	return "", fmt.Errorf("username required: use --user, set XPC_USER, or add 'user' to ~/.xpc.yaml")
}

// ResolvePassword returns the password for host from (in priority order):
// XPC_<HOST>_PASSWORD env var, XPC_PASSWORD env var, or an interactive terminal prompt.
func ResolvePassword(host string) (string, error) {
	if p := os.Getenv("XPC_" + EnvSafe(host) + "_PASSWORD"); p != "" {
		return p, nil
	}
	if p := os.Getenv("XPC_PASSWORD"); p != "" {
		return p, nil
	}
	return promptPassword(fmt.Sprintf("Password for %s: ", host))
}

// ResolveFTPURL ensures the FTP URL carries credentials, resolving the password
// securely rather than requiring it embedded in the raw URL string.
func ResolveFTPURL(rawURL, flagUser string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid FTP URL %q: %w", rawURL, err)
	}
	if u.Scheme != "ftp" {
		return "", fmt.Errorf("expected ftp:// URL, got scheme %q", u.Scheme)
	}

	username := u.User.Username()
	existingPass, hasPass := u.User.Password()

	if username == "" {
		username, err = ResolveUser(flagUser)
		if err != nil {
			return "", err
		}
	}

	var password string
	if hasPass && existingPass != "" {
		fmt.Fprintln(os.Stderr, "Warning: embedding a password in an FTP URL is not recommended.\n"+
			"         Use XPC_FTP_PASSWORD (or XPC_PASSWORD) instead and omit it from the URL.")
		// URL already contains credentials; return as-is.
	} else {
		if p := os.Getenv("XPC_FTP_PASSWORD"); p != "" {
			password = p
		} else if p := os.Getenv("XPC_PASSWORD"); p != "" {
			password = p
		} else {
			password, err = promptPassword(fmt.Sprintf("FTP password for %s@%s: ", username, u.Hostname()))
			if err != nil {
				return "", err
			}
		}
		u.User = url.UserPassword(username, password)
	}

	return u.String(), nil
}

// promptPassword displays prompt on stderr and reads a password with echo
// disabled. Returns a clear error in non-interactive (CI) environments.
func promptPassword(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf(
			"no password found in environment and stdin is not a terminal.\n" +
				"Set XPC_PASSWORD (or XPC_FTP_PASSWORD) before running, or add it to a .env file.")
	}
	fmt.Fprint(os.Stderr, prompt)
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("password cannot be empty")
	}
	return string(raw), nil
}
