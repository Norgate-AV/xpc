package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Norgate-AV/xpc/internal/converter"
	"github.com/Norgate-AV/xpc/internal/credentials"
	"github.com/Norgate-AV/xpc/internal/transfer"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:          "xpc",
	Short:        "Convert a Crestron WebXPanel (Flash) project to XPanel Air",
	SilenceUsage: true, // Don't show usage on runtime errors
	Long: `xpc converts a Crestron WebXPanel (Flash) project to XPanel Air format.

Two modes — detected from flags, no subcommand needed:

  REMOTE  --host HOST  Connect via SFTP, pull the project from the processor,
                       convert it, then push it back — no Crestron Toolbox needed.

  LOCAL   --dir PATH   Convert a project already on your local machine.

Passwords are never accepted on the command line.  Use environment variables
or a .env file (auto-loaded from CWD or ~/.xpc.env) instead:

  XPC_USER                   Username for SSH / FTP connections
  XPC_PASSWORD               Password (all connections)
  XPC_<HOST>_PASSWORD        Per-host password  (e.g. XPC_192_168_1_1_PASSWORD)
  XPC_FTP_PASSWORD           FTP-specific password override
  XPC_TOOL_PATH              Path to xpanelconversiontool.cli.exe

Non-sensitive settings (host, user, tool_path, ctrl_path, report_dir) can be
stored in ~/.xpc.yaml so you rarely need any flags at all.

Examples:
  # Remote: pull from processor, convert, push back
  xpc --host 192.168.1.100 --user admin --cnx-ip-id 03

  # Remote with credentials from .env file
  echo "XPC_PASSWORD=secret" > .env
  xpc --host 192.168.1.100

  # Local: convert a project already on disk
  xpc --dir ./my-xpanel-project

  # Local with explicit output directory
  xpc --dir ./my-xpanel-project --out ./converted-output`,
	RunE: runXPC,
}

// Execute is called by main.main().
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	f := rootCmd.Flags()

	// --- Mode (mutually exclusive; one is required) --------------------------
	f.StringP("host", "H", "", "Processor hostname or IP for remote mode (SFTP — preferred over FTP)")
	f.String("ftp", "", "Fusion server FTP URL for remote mode, e.g. ftp://user@host/path  (no password)")
	f.StringP("dir", "d", "", "Local source directory for local-only mode")

	// --- Connection (remote mode) --------------------------------------------
	f.StringP("user", "u", "", "Username  [env: XPC_USER | config: user]")
	f.Int("ssh-port", 22, "SSH/SFTP port for controller connections")
	f.Bool("insecure", false, "Skip SSH host key verification  (not recommended outside dev environments)")
	f.String("ctrl-path", "", `Remote directory path on the processor  (SFTP only; default: \html\)`)

	// --- Output --------------------------------------------------------------
	f.StringP("out", "o", "", "Output directory for converted files  (default: same as --dir)")
	f.String("report", "", "Directory for conversion reports  (default: OS temp dir)")
	f.BoolP("force", "f", false, "Overwrite existing files")
	f.Bool("keep-work-dir", false, "Keep the intermediate working directory after completion")

	// --- HTML conversion args ------------------------------------------------
	f.String("html-args", "", "Pre-existing HTML args XML file  (skips flag-based generation)")
	f.Bool("fusion", false, "Target a Fusion server  (default: false = Controller)")
	f.String("ctrl-html-path", "", `Control HTML path on the device  (e.g. \WebXpanel.c3prj\)`)
	f.String("mac-installer", "", "URL to the macOS CH5 installer package")
	f.String("win-installer", "", "URL to the Windows CH5 installer package")

	// --- CNX connection args (embedded in the converted project) -------------
	f.String("cnx-args", "", "Pre-existing CNX args XML file  (skips flag-based generation)")
	f.String("cnx-host", "", "Controller hostname/IP to embed  (default: value of --host)")
	f.Int("cnx-port", 41794, "Controller port to embed")
	f.String("cnx-ip-id", "", "IP ID in hex to embed, e.g. 03  (range: 03–FE)")
	f.Bool("cnx-ssl", false, "Enable SSL in the embedded connection settings")

	// --- Config / env --------------------------------------------------------
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file  (default: $HOME/.xpc.yaml)")
	rootCmd.PersistentFlags().String("env-file", "", "path to a .env file to load")
	rootCmd.PersistentFlags().BoolP("verbose", "V", false, "show progress messages and tool output")
}

// runXPC is the single entry point for both remote and local modes.
func runXPC(cmd *cobra.Command, _ []string) error {
	// --- Resolve mode --------------------------------------------------------
	host := flagOrConfig(cmd, "host", "host")
	ftpURL, _ := cmd.Flags().GetString("ftp")
	source, _ := cmd.Flags().GetString("dir")
	verbose, _ := cmd.Flags().GetBool("verbose")

	isRemote := host != "" || ftpURL != ""

	switch {
	case host != "" && ftpURL != "":
		return fmt.Errorf("--host and --ftp are mutually exclusive")
	case isRemote && source != "":
		return fmt.Errorf("--source cannot be combined with --host or --ftp")
	case !isRemote && source == "":
		return fmt.Errorf("specify --source <path> (local) or --host <IP> (remote)")
	}

	// --- Shared settings -----------------------------------------------------
	report := flagOrConfig(cmd, "report", "report_dir")
	if report == "" {
		report = converter.DefaultReportDir()
	}
	force, _ := cmd.Flags().GetBool("force")
	keepWorkDir, _ := cmd.Flags().GetBool("keep-work-dir")
	dest := flagOrConfig(cmd, "out", "out_dir")

	// --- Working directories -------------------------------------------------
	var sourceDir, convertedDir string
	var workDir string

	if isRemote {
		ts := time.Now().Format("20060102-150405")
		workDir = filepath.Join(os.TempDir(), "xpc-work", ts)
		sourceDir = filepath.Join(workDir, "downloaded")
		if err := os.MkdirAll(sourceDir, 0o700); err != nil {
			return fmt.Errorf("creating working directory: %w", err)
		}
		if !keepWorkDir {
			defer func() {
				if verbose {
					fmt.Fprintf(os.Stderr, "Cleaning up %s\n", workDir)
				}
				_ = os.RemoveAll(workDir)
			}()
		}
		if dest != "" {
			convertedDir = dest
		} else {
			convertedDir = filepath.Join(workDir, "converted")
		}
	} else {
		var pathErr error
		sourceDir, convertedDir, pathErr = absLocalPaths(source, dest)
		if pathErr != nil {
			return pathErr
		}

		// In-place conversion: xpanelconversiontool.cli.exe deletes ALL files in
		// the source directory during its clean step before writing output.
		// To prevent destroying the project, copy the source to a temp directory
		// and let the tool read from the copy while writing to the original.
		if sourceDir == convertedDir {
			if verbose {
				fmt.Fprintln(os.Stderr, "▸ Creating working copy for in-place conversion...")
			}
			tmpSrc, copyErr := copyToTemp(sourceDir)
			if copyErr != nil {
				return fmt.Errorf("creating working copy: %w", copyErr)
			}
			if !keepWorkDir {
				defer func() {
					if verbose {
						fmt.Fprintf(os.Stderr, "Cleaning up working copy %s\n", tmpSrc)
					}
					_ = os.RemoveAll(tmpSrc)
				}()
			}
			sourceDir = tmpSrc
			// convertedDir remains the original directory — converted files land there.
		}
	}

	// --- Step 1: get (remote only) -------------------------------------------
	var rc *transfer.Config
	if isRemote {
		var rcErr error
		rc, rcErr = buildRemoteConfig(cmd, host, ftpURL)
		if rcErr != nil {
			return rcErr
		}
		rc.Verbose = verbose
		if verbose {
			fmt.Fprintln(os.Stderr, "▸ Downloading project from processor...")
		}
		if err := rc.Download(sourceDir); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	}

	// --- Step 2: convert -----------------------------------------------------
	if verbose {
		fmt.Fprintln(os.Stderr, "▸ Converting project...")
	}

	htmlArgsPath, _ := cmd.Flags().GetString("html-args")
	fusion, _ := cmd.Flags().GetBool("fusion")
	ctrlHTMLPath, _ := cmd.Flags().GetString("ctrl-html-path")
	macInstaller, _ := cmd.Flags().GetString("mac-installer")
	winInstaller, _ := cmd.Flags().GetString("win-installer")
	htmlArgsFile, htmlCleanup, err := converter.ResolveHTMLArgs(converter.HTMLConversionArgs{
		PassthroughFile:      htmlArgsPath,
		IsFusionServer:       fusion,
		ControlHtmlPath:      ctrlHTMLPath,
		MacInstallerLink:     macInstaller,
		WindowsInstallerLink: winInstaller,
	})
	if err != nil {
		return err
	}
	if htmlCleanup != nil {
		defer htmlCleanup()
	}

	// Default --cnx-host to --host when not explicitly set
	cnxHost, _ := cmd.Flags().GetString("cnx-host")
	if cnxHost == "" {
		cnxHost = host
	}
	cnxArgsPath, _ := cmd.Flags().GetString("cnx-args")
	cnxPort, _ := cmd.Flags().GetInt("cnx-port")
	cnxIpId, _ := cmd.Flags().GetString("cnx-ip-id")
	cnxSSL, _ := cmd.Flags().GetBool("cnx-ssl")
	cnxArgsFile, cnxCleanup, err := converter.ResolveCNXArgs(converter.CNXConnectionArgs{
		PassthroughFile: cnxArgsPath,
		Host:            cnxHost,
		Port:            cnxPort,
		IpId:            cnxIpId,
		EnableSSL:       cnxSSL,
	})
	if err != nil {
		return err
	}
	if cnxCleanup != nil {
		defer cnxCleanup()
	}

	convertArgs := []string{"convert"}
	if force {
		convertArgs = append(convertArgs, "-overwrite")
	}
	convertArgs = append(convertArgs, sourceDir, convertedDir, report, htmlArgsFile)
	if cnxArgsFile != "" {
		convertArgs = append(convertArgs, fmt.Sprintf("--cnx=%s", cnxArgsFile))
	}
	if err := converter.Run(verbose, convertArgs...); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Remove empty directories left behind by the tool (it deletes individual
	// Flash files but does not remove the now-empty subdirectories).
	if !isRemote {
		if cleanErr := removeEmptyDirs(convertedDir); cleanErr != nil && verbose {
			fmt.Fprintf(os.Stderr, "Warning: could not remove empty directories: %v\n", cleanErr)
		}
	}

	// --- Step 3: put (remote only) -------------------------------------------
	if isRemote {
		if verbose {
			fmt.Fprintln(os.Stderr, "▸ Uploading converted project to processor...")
		}
		if err := rc.Upload(convertedDir); err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}
		if verbose {
			fmt.Fprintln(os.Stderr, "✓ Done.")
		}
	} else {
		if verbose {
			fmt.Fprintf(os.Stderr, "✓ Converted project written to: %s\n", convertedDir)
		}
	}

	return nil
}

// buildRemoteConfig constructs a transfer.Config from command flags.
// Credentials are resolved securely — never from a CLI flag.
func buildRemoteConfig(cmd *cobra.Command, host, ftpURL string) (*transfer.Config, error) {
	sshPort, _ := cmd.Flags().GetInt("ssh-port")
	insecure, _ := cmd.Flags().GetBool("insecure")
	ctrlPath := flagOrConfig(cmd, "ctrl-path", "ctrl_path")
	if ctrlPath == "" {
		ctrlPath = `\html\`
	}

	rc := &transfer.Config{
		CtrlPath: ctrlPath,
		Insecure: insecure,
	}

	if host != "" {
		// SFTP mode
		user, err := credentials.ResolveUser(flagOrConfig(cmd, "user", "user"))
		if err != nil {
			return nil, err
		}
		pass, err := credentials.ResolvePassword(host)
		if err != nil {
			return nil, err
		}
		rc.Host = host
		rc.Port = sshPort
		rc.User = user
		rc.Pass = pass
	} else {
		// FTP mode — resolve credentials into the URL
		flagUser := flagOrConfig(cmd, "user", "user")
		resolvedURL, err := credentials.ResolveFTPURL(ftpURL, flagUser)
		if err != nil {
			return nil, err
		}
		rc.FtpURL = resolvedURL
	}

	return rc, nil
}

// flagOrConfig returns the flag value if it was explicitly set on the command
// line, otherwise falls back to the viper config key (which covers config file
// values and env vars bound via viper.AutomaticEnv).
func flagOrConfig(cmd *cobra.Command, flagName, configKey string) string {
	if cmd.Flags().Changed(flagName) {
		v, _ := cmd.Flags().GetString(flagName)
		return v
	}
	return viper.GetString(configKey)
}

// --- Path helpers ------------------------------------------------------------

// absLocalPaths resolves source and optional dest to absolute paths.
// When dest is empty the default is in-place conversion (dest == source).
// This ensures the underlying conversion tool always receives fully-qualified
// paths (it rejects relative URIs with "Invalid URI" errors).
//
// When source == dest, runXPC transparently copies the source to a temp
// directory so the conversion tool can safely delete from the copy while
// writing converted output back to the original location.
func absLocalPaths(source, dest string) (absSource, absDest string, err error) {
	absSource, err = filepath.Abs(source)
	if err != nil {
		return "", "", fmt.Errorf("resolving source path: %w", err)
	}
	if dest != "" {
		absDest, err = filepath.Abs(dest)
		if err != nil {
			return "", "", fmt.Errorf("resolving output path: %w", err)
		}
	} else {
		absDest = absSource // default: in-place
	}
	return absSource, absDest, nil
}

// copyToTemp recursively copies src into a new OS temp directory and returns
// its path. The caller is responsible for removing it when done.
func copyToTemp(src string) (string, error) {
	dir, err := os.MkdirTemp("", "xpc-work-*")
	if err != nil {
		return "", err
	}
	if err := copyDir(src, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// copyDir recursively copies all files and subdirectories from src into dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// removeEmptyDirs walks dir bottom-up and removes any empty subdirectories.
// This cleans up directories the Crestron tool empties (by deleting their
// files) but does not itself remove.  The root dir is never removed.
func removeEmptyDirs(dir string) error {
	var dirs []string
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != dir {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return err
	}
	// Process deepest directories first so parents are checked after children.
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(dirs[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Config init -------------------------------------------------------------

func initConfig() {
	// .env loading — explicit flag wins, else auto-discover
	if envFile, _ := rootCmd.PersistentFlags().GetString("env-file"); envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load env file %q: %v\n", envFile, err)
		}
	} else {
		if err := godotenv.Load(".env"); err != nil {
			if home, err2 := os.UserHomeDir(); err2 == nil {
				_ = godotenv.Load(filepath.Join(home, ".xpc.env"))
			}
		}
	}

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".xpc")
	}

	_ = viper.BindEnv("tool_path", "XPC_TOOL_PATH")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config:", viper.ConfigFileUsed())
	}
}
