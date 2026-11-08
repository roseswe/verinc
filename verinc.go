// verinc - Version increment tool
// Build with: go build -o verinc
//
// CODE REVIEW FINDINGS AND FIXES
// ==============================
// The following defects were found during a full review and have been
// fixed in this revision (see test program verinc_test.go):
//
// SECURITY / CORRECTNESS
//  1. fnProcessFile used os.Create() which truncates the target before
//     writing. A mid-write failure (disk full, EIO, signal) destroyed the
//     original. Replaced with an atomic "write temp file -> rename" scheme
//     (fnWriteFileAtomically).
//  2. os.Create() applied mode 0666&umask, silently dropping the
//     executable bit / custom ACLs. The original file mode is now copied
//     to the temp file before the rename.
//  3. os.Create() follows symlinks but rename() replaces the symlink with
//     a regular file - silently converting symlinks into real files.
//     Symlinks are now resolved via filepath.EvalSymlinks and the real
//     target is atomically replaced.
//  4. bufio.Scanner.ScanLines() strips the trailing "\r\n"; the writer
//     re-appended only "\n". CRLF files were silently converted to LF on
//     every run (data corruption on Windows sources). Switched to
//     bufio.Reader.ReadString('\n') which keeps the original terminator.
//  5. The check `err.Error() == "no files specified"` could never match
//     (the actual message contains trailing text), so the tool exited with
//     code 2 instead of 3. A package-level sentinel error `errNoFiles` is
//     now used.
//  6. `-h` / `--help` caused flag.Parse to return flag.ErrHelp, which was
//     reported as "invalid command line arguments". ErrHelp is now
//     translated into the help view.
//  7. The line-selection token sets in fnProcessLine and fnGetVersion
//     were inconsistent (one required a trailing colon, one did not), so
//     `--get` could return a version for a line that `--get`-less mode
//     would not touch. Unified in fnIsVersionLine.
//
// FEATURES
//  8. New option -n / --nowrap: disables the "patch >= 10 -> minor+1,
//     patch=1" wrap rule. With --nowrap, 1.7.9 -> 1.7.10 (instead of
//     1.8.1). --minor / --major are unaffected; they still explicitly
//     reset patch to 1 (documented behaviour).
//
// ENHANCEMENT SUGGESTIONS (not yet implemented)
//   * --dry-run: report what would change without writing.
//   * --backup[=suffix]: keep the original (e.g. main.pas.bak).
//   * --recursive: walk a directory and process matching files.
//   * --fail-fast / --continue: choose the behaviour for multi-file runs
//     (today: stop at the first hard error).
//   * Advisory file locking to serialise concurrent invocations.
//   * --allow-custom-regex: user-supplied version pattern.
//   * Configurable version scheme (e.g. MAJOR.MINOR.PATCH.BUILD).
//   * Handle EXDEV (cross-device rename) on exotic mounts by falling back
//     to copy+rename.
//   * Structured (JSON) logging as an alternative to the emoji output.
//
// Compatibility: tested build targets are win32/win64 (amd64), linux
// 386/amd64 and macOS (amd64/arm64). Uses only the Go standard library.
//
// $Id: verinc.go,v 1.12 2026/11/08 22:45:12 ralph Exp $

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Global version of verinc (increment this manually when you release a new build).
// Example: 1.0.5 -> 1.0.2
const (
	szVerincVersion = "2.2.1" // _VERSION
)

// Exit codes (documented in --help):
// 0  - success
// 1  - generic error
// 2  - invalid command line arguments
// 3  - no files provided
// 4  - file open/read error
// 5  - no _VERSION line found in any file
// 6  - version parse error
// 7  - file write error

const (
	iExitOK              = 0
	iExitGenericError    = 1
	iExitBadArguments    = 2
	iExitNoFiles         = 3
	iExitFileReadError   = 4
	iExitNoVersionLines  = 5
	iExitVersionParseErr = 6
	iExitFileWriteError  = 7
)

// errNoFiles is a sentinel so main can distinguish "no files" (exit 3)
// from any other argument error (exit 2) without fragile string compares.
var errNoFiles = errors.New("no files specified")

type tConfig struct {
	bVerbose     bool
	bShowHelp    bool
	bShowUsage   bool
	bShowVersion bool
	bGet         bool
	bMinor       bool
	bMajor       bool
	bNoWrap      bool // --nowrap: do not wrap patch 9 -> 1 at 10
	aszFiles     []string
}

var (
	reVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
)

func fnMainUsage() {
	szExeName := filepath.Base(os.Args[0])
	fmt.Fprintf(flag.CommandLine.Output(), "verinc - Increment or get version strings in source files\n")
	fmt.Fprintf(flag.CommandLine.Output(), "         https://github.com/roseswe/verinc\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Usage:\n  %s [options] file1 [file2 ...]\n\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -?, -h, --help, --usage   Show this detailed help and exit\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -V, --version             Show verinc program version and exit\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -v, --verbose             Enable verbose output (default: false)\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -g, --get                 Return first matching version string to stdout\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -m, --minor               Bump minor version instead of patch\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -j, --major               Bump major version (reset minor and patch to 0 and 1)\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -n, --nowrap              Bump PATCH without wrapping (1.7.9 -> 1.7.10)\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Version rules:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * Version format: MAJOR.MINOR.PATCH (e.g. 3.19.6).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * Default: increment PATCH (3.19.6 -> 3.19.7).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * With -m/--minor: increment MINOR and reset PATCH to 1 (3.19.6 -> 3.20.1).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * With -j/--major: increment MAJOR and reset MINOR/PATCH to 0/1 (3.19.6 -> 4.0.1).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * If PATCH would reach 10, reset PATCH to 1 and increment MINOR\n")
	fmt.Fprintf(flag.CommandLine.Output(), "    (3.20.9 -> 3.21.1). Use -n/--nowrap to disable this wrap rule\n")
	fmt.Fprintf(flag.CommandLine.Output(), "    (3.20.9 -> 3.20.10).\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Line selection:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * Only lines containing a recognised version token are processed\n")
	fmt.Fprintf(flag.CommandLine.Output(), "    (\"_VERSION\", \"ProductVersion\", \"FileVersion\", \"\\\"Version\\\"\").\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * The first MAJOR.MINOR.PATCH in such a line is updated.\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Exit codes:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  0  Success\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  1  Generic error\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  2  Invalid command line arguments\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  3  No files provided\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  4  File open/read error\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  5  No _VERSION line found in any file\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  6  Version parse error\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  7  File write error\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Examples:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  %s main.pas\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "  %s -v ~/src/prj/mpscan/mpscan.pas\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "  %s -m --verbose ~/tp/vir/rms.pas\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "  %s -n main.pas          # 1.7.9 -> 1.7.10\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "  %s --get file.go\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "  VER=$(%s -g main.pas)  # Useful in Makefiles/Bash scripts\n", szExeName)
} // end:fnMainUsage

func fnParseArgs() (*tConfig, error) {
	stCfg := &tConfig{}

	// Pre-scan raw argv for help aliases the flag package does not know
	// about (e.g. -?, --usage, -H, -U). This must run BEFORE Parse: the
	// flag package would otherwise fail with "flag provided but not
	// defined: -?". "-h"/"--help"/"-help" are also caught by ErrHelp
	// below, but listing them here keeps behaviour uniform.
	for _, szArg := range os.Args[1:] {
		switch szArg {
		case "-?", "-h", "--help", "-help", "-H", "--usage", "-usage", "-U", "help", "usage":
			stCfg.bShowHelp = true
			return stCfg, nil
		}
	} // end:for

	// Create a custom FlagSet to allow our own usage and aliases.
	stFlagSet := flag.NewFlagSet("verinc", flag.ContinueOnError)
	stFlagSet.SetOutput(os.Stdout)
	stFlagSet.Usage = fnMainUsage

	// Primary flags (long + short names sharing the same variables).
	pbVerbose := stFlagSet.Bool("verbose", false, "enable verbose output")
	pbVerboseShort := stFlagSet.Bool("v", false, "enable verbose output (short)")

	pbMinor := stFlagSet.Bool("minor", false, "bump minor version instead of patch")
	pbMinorShort := stFlagSet.Bool("m", false, "bump minor version instead of patch (short)")

	pbMajor := stFlagSet.Bool("major", false, "bump major version")
	pbMajorShort := stFlagSet.Bool("j", false, "bump major version (short)")

	pbGet := stFlagSet.Bool("get", false, "return first matching version string to stdout")
	pbGetShort := stFlagSet.Bool("g", false, "return first matching version string to stdout (short)")

	pbNoWrap := stFlagSet.Bool("nowrap", false, "do not wrap patch at 10")
	pbNoWrapShort := stFlagSet.Bool("n", false, "do not wrap patch at 10 (short)")

	// Version flag (long + short).
	pbVersion := stFlagSet.Bool("version", false, "show verinc program version and exit")
	pbVersionShort := stFlagSet.Bool("V", false, "show verinc program version and exit (short)")

	// Parse known flags. flag.ErrHelp is a *request* for help, not an
	// argument error - translate it accordingly.
	if err := stFlagSet.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			stCfg.bShowHelp = true
			return stCfg, nil
		}
		return nil, err
	} // end:if

	// Merge long/short versions.
	stCfg.bVerbose = *pbVerbose || *pbVerboseShort
	stCfg.bGet = *pbGet || *pbGetShort
	stCfg.bMinor = *pbMinor || *pbMinorShort
	stCfg.bMajor = *pbMajor || *pbMajorShort
	stCfg.bNoWrap = *pbNoWrap || *pbNoWrapShort
	stCfg.bShowVersion = *pbVersion || *pbVersionShort

	// Remaining arguments after flag parsing are files.
	stCfg.aszFiles = stFlagSet.Args()

	if len(stCfg.aszFiles) == 0 && !stCfg.bShowHelp && !stCfg.bShowVersion && !stCfg.bGet {
		return nil, errNoFiles
	}

	return stCfg, nil
} // end:fnParseArgs

func fnInfo(bVerbose bool, szMsg string, aArgs ...interface{}) {
	if !bVerbose {
		return
	}
	fmt.Fprintf(os.Stdout, "ℹ️ [!] "+szMsg+"\n", aArgs...)
} // end:fnInfo

func fnError(szMsg string, aArgs ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ [!] "+szMsg+"\n", aArgs...)
} // end:fnError

// fnBumpVersion bumps MAJOR.MINOR.PATCH according to the supplied flags.
// With bNoWrap=false (default) patch 9 -> 1 and MINOR is incremented.
// With bNoWrap=true the patch is incremented without wrapping
// (1.7.9 -> 1.7.10). --minor/--major are unaffected by bNoWrap and always
// reset patch to 1.
func fnBumpVersion(szVersion string, bMinor bool, bMajor bool, bNoWrap bool) (string, error) {
	aszParts := strings.Split(szVersion, ".")
	if len(aszParts) != 3 {
		return "", errors.New("invalid version format")
	}

	iMajor, err := strconv.Atoi(aszParts[0])
	if err != nil {
		return "", err
	}
	iMinor, err := strconv.Atoi(aszParts[1])
	if err != nil {
		return "", err
	}
	iPatch, err := strconv.Atoi(aszParts[2])
	if err != nil {
		return "", err
	}

	if bMajor {
		// Major bump: MAJOR.MINOR.PATCH -> (MAJOR+1).0.1
		iMajor++
		iMinor = 0
		iPatch = 1
	} else if bMinor {
		// Minor bump: MAJOR.MINOR.PATCH -> MAJOR.(MINOR+1).1
		iMinor++   // e.g. 3.42.6 -> 3.43.1 (not 3.10.1: use -j for that)
		iPatch = 1
	} else {
		// Patch bump with overflow rule unless --nowrap is active.
		iPatch++
		if !bNoWrap && iPatch >= 10 {
			iPatch = 1
			iMinor++
		}
	} // end:if bMajor/else if bMinor/else

	szNew := fmt.Sprintf("%d.%d.%d", iMajor, iMinor, iPatch)
	return szNew, nil
} // end:fnBumpVersion

// fnIsVersionLine reports whether the line should be considered for
// version matching. The token set is intentionally identical for --get
// and the increment path so both operate on the same lines.
func fnIsVersionLine(szLine string) bool {
	return strings.Contains(szLine, "_VERSION") ||
		strings.Contains(szLine, "ProductVersion") ||
		strings.Contains(szLine, "FileVersion") ||
		strings.Contains(szLine, "\"Version\"")
} // end:fnIsVersionLine

// fnProcessLine replaces the first MAJOR.MINOR.PATCH on a matching line.
// The line is expected to still carry its original terminator so that
// CRLF vs LF is preserved when it is written back.
func fnProcessLine(szFile string, szLine string, stCfg *tConfig, pbChanged *bool) (string, error) {

	if !fnIsVersionLine(szLine) {
		return szLine, nil
	}

	aszMatch := reVersionPattern.FindStringSubmatch(szLine)
	if len(aszMatch) != 4 {
		return szLine, fmt.Errorf("no version pattern found: %v (len=%d): %q", aszMatch, len(aszMatch), szLine)
	}

	szOldVersion := aszMatch[0]
	szNewVersion, err := fnBumpVersion(szOldVersion, stCfg.bMinor, stCfg.bMajor, stCfg.bNoWrap)
	if err != nil {
		return szLine, err
	}

	szNewLine := strings.Replace(szLine, szOldVersion, szNewVersion, 1)
	*pbChanged = true

	if stCfg.bVerbose {
		fmt.Fprintf(os.Stdout,
			"ℹ️ [!] verinc: %s: %s -> %s\n",
			szFile, szOldVersion, szNewVersion)
	}

	return szNewLine, nil
} // end:fnProcessLine

func fnGetVersion(szFile string) (string, error) {
	fIn, err := os.Open(szFile)
	if err != nil {
		return "", err
	}
	defer fIn.Close()

	stScanner := bufio.NewScanner(fIn)
	for stScanner.Scan() {
		szLine := stScanner.Text()

		if !fnIsVersionLine(szLine) {
			continue
		}

		aszMatch := reVersionPattern.FindStringSubmatch(szLine)
		if len(aszMatch) == 4 {
			return aszMatch[0], nil
		}
	} // end:for
	if err := stScanner.Err(); err != nil {
		return "", err
	}

	return "", errors.New("no version pattern found")
} // end:fnGetVersion

// fnProcessFile reads, modifies and atomically replaces szPath. On any
// error the original file remains untouched.
func fnProcessFile(szPath string, stCfg *tConfig) (bool, int, error) {
	fnInfo(stCfg.bVerbose, "Processing file: %s", szPath)

	// Resolve symlinks so the rename step replaces the real target
	// instead of converting the symlink into a regular file.
	realPath, err := filepath.EvalSymlinks(szPath)
	if err != nil {
		return false, iExitFileReadError, err
	}
	if stCfg.bVerbose && realPath != szPath {
		fnInfo(stCfg.bVerbose, "Following symlink: %s -> %s", szPath, realPath)
	}

	fIn, err := os.Open(realPath)
	if err != nil {
		return false, iExitFileReadError, err
	}
	defer fIn.Close()

	var aszLines []string
	bAnyChanged := false

	// Use bufio.Reader.ReadString rather than bufio.Scanner: Scanner
	// strips CRLF and would cause silent CRLF -> LF corruption.
	reader := bufio.NewReader(fIn)
	for {
		szLine, rerr := reader.ReadString('\n')
		if len(szLine) > 0 {
			bChanged := false
			szNewLine, perr := fnProcessLine(szPath, szLine, stCfg, &bChanged)
			if perr != nil && stCfg.bVerbose {
				fnInfo(stCfg.bVerbose, "Version processing warning in %s: %v", szPath, perr)
			}
			if bChanged {
				bAnyChanged = true
			}
			aszLines = append(aszLines, szNewLine)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return false, iExitFileReadError, rerr
		}
	} // end:for

	if !bAnyChanged {
		fnInfo(stCfg.bVerbose, "No _VERSION/ProductVersion/FileVersion line updated in %s", szPath)
		return false, iExitNoVersionLines, nil
	}

	if werr := fnWriteFileAtomically(realPath, aszLines); werr != nil {
		return false, iExitFileWriteError, werr
	}

	fnInfo(stCfg.bVerbose, "Updated file: %s", szPath)
	return true, iExitOK, nil
} // end:fnProcessFile

// fnWriteFileAtomically writes aszLines (which include their original
// line terminators) to a temporary file in the same directory and then
// renames it over szPath. This preserves the original file on error and
// keeps its permission bits.
func fnWriteFileAtomically(szPath string, aszLines []string) error {
	dir := filepath.Dir(szPath)

	tmp, err := os.CreateTemp(dir, ".verinc-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		_ = tmp.Close()
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	// Copy the original mode (exec bit, group/other bits).
	if fi, statErr := os.Stat(szPath); statErr == nil {
		if chErr := tmp.Chmod(fi.Mode().Perm()); chErr != nil {
			return chErr
		}
	}

	stWriter := bufio.NewWriter(tmp)
	for _, szLine := range aszLines {
		if _, werr := stWriter.WriteString(szLine); werr != nil {
			return werr
		}
	} // end:for
	if err := stWriter.Flush(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// On Windows this uses MoveFileEx(MOVEFILE_REPLACE_EXISTING) under
	// the hood; on POSIX it is rename(2). Both are atomic within a
	// filesystem. Cross-device renames fail with EXDEV - since the temp
	// file is created next to the target this should not happen in
	// practice.
	if err := os.Rename(tmpName, szPath); err != nil {
		return err
	}

	success = true
	return nil
} // end:fnWriteFileAtomically

func main() {
	flag.CommandLine.Usage = fnMainUsage

	stCfg, err := fnParseArgs()
	if err != nil {
		if errors.Is(err, errNoFiles) {
			fnError("No files specified.")
			os.Exit(iExitNoFiles)
		}
		fnError("Invalid command line arguments: %v", err)
		os.Exit(iExitBadArguments)
	} // end:if

	if stCfg.bShowHelp {
		fnMainUsage()
		os.Exit(iExitOK)
	}

	if stCfg.bShowVersion {
		fmt.Fprintf(os.Stdout, "verinc version %s\n@(#) $Id: verinc.go,v 1.12 2026/11/08 22:45:12 ralph Exp $\n", szVerincVersion)
		os.Exit(iExitOK)
	}

	if stCfg.bGet {
		// --get mode: return first matching version string to stdout.
		if len(stCfg.aszFiles) == 0 {
			fnError("No files specified.")
			os.Exit(iExitNoFiles)
		}

		for _, szPath := range stCfg.aszFiles {
			szVersion, gerr := fnGetVersion(szPath)
			if gerr != nil {
				fnError("Error processing file %s: %v", szPath, gerr)
				os.Exit(iExitNoVersionLines)
			}
			fmt.Fprintf(os.Stdout, "%s\n", szVersion)
		} // end:for
		os.Exit(iExitOK)
	} // end:if bGet

	if len(stCfg.aszFiles) == 0 {
		fnError("No files specified.")
		os.Exit(iExitNoFiles)
	}

	bAnyFileChanged := false
	iLastExitCode := iExitOK

	for _, szPath := range stCfg.aszFiles {
		bChanged, iCode, perr := fnProcessFile(szPath, stCfg)
		if perr != nil && iCode != iExitNoVersionLines {
			fnError("Error processing file %s: %v", szPath, perr)
			os.Exit(iCode)
		}
		if bChanged {
			bAnyFileChanged = true
		}
		if iCode != iExitOK {
			iLastExitCode = iCode
		}
	} // end:for

	if !bAnyFileChanged {
		fnError("No _VERSION/ProductVersion/FileVersion lines updated in any file.")
		os.Exit(iExitNoVersionLines)
	}

	os.Exit(iLastExitCode)
} // end:main