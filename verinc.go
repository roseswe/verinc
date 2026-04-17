// verinc - Version increment tool
// Build with: go build -o verinc

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Global version of verinc (increment this manually when you release a new build).
// Example: 1.0.5 -> 1.0.2
const (
	szVerincVersion = "2.1.1" // _VERSION
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

type tConfig struct {
	bVerbose     bool
	bShowHelp    bool
	bShowUsage   bool
	bShowVersion bool
	bGet         bool
	bMinor       bool
	bMajor       bool
	aszFiles     []string
}

var (
	reVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
)

func fnMainUsage() {
	szExeName := filepath.Base(os.Args[0])
	fmt.Fprintf(flag.CommandLine.Output(), "verinc - Increment version strings in source files\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Usage:\n  %s [options] file1 [file2 ...]\n\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -?, -h, --help, --usage   Show this detailed help and exit\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -V, --version             Show verinc program version and exit\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -v, --verbose             Enable verbose output (default: false)\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -g, --get                 Return first matching version string to stdout\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -m, --minor               Bump minor version instead of patch\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  -j, --major               Bump major version (reset minor and patch to 0 and 1)\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Version rules:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * Version format: MAJOR.MINOR.PATCH (e.g. 3.19.6).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * Default: increment PATCH (3.19.6 -> 3.19.7).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * With -m/--minor: increment MINOR and reset PATCH to 1 (3.19.6 -> 3.20.1).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * With -j/--major: increment MAJOR and reset MINOR/PATCH to 0/1 (3.19.6 -> 4.0.1).\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * If PATCH would reach 10, reset PATCH to 1 and increment MINOR\n")
	fmt.Fprintf(flag.CommandLine.Output(), "    (3.20.5 -> 3.20.6).\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Line selection:\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  * Only lines containing the token \"_VERSION\" are processed.\n")
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
	fmt.Fprintf(flag.CommandLine.Output(), "  %s --get file.go\n", szExeName)
	fmt.Fprintf(flag.CommandLine.Output(), "  VER=$(%s -g main.pas)  # Useful in Makefiles/Bash scripts\n", szExeName)
}

func fnParseArgs() (*tConfig, error) {
	stCfg := &tConfig{}

	// Create a custom FlagSet to allow our own usage and aliases.
	stFlagSet := flag.NewFlagSet("verinc", flag.ContinueOnError)
	stFlagSet.SetOutput(os.Stdout)
	stFlagSet.Usage = fnMainUsage

	// Primary flags (long + short names sharing same variables)
	pbVerbose := stFlagSet.Bool("verbose", false, "enable verbose output")
	pbVerboseShort := stFlagSet.Bool("v", false, "enable verbose output (short)")

	pbMinor := stFlagSet.Bool("minor", false, "bump minor version instead of patch")
	pbMinorShort := stFlagSet.Bool("m", false, "bump minor version instead of patch (short)")

	pbMajor := stFlagSet.Bool("major", false, "bump major version")
	pbMajorShort := stFlagSet.Bool("j", false, "bump major version (short)")

	pbGet := stFlagSet.Bool("get", false, "return first matching version string to stdout")
	pbGetShort := stFlagSet.Bool("g", false, "return first matching version string to stdout (short)")

	// Version flag (long + short)
	pbVersion := stFlagSet.Bool("version", false, "show verinc program version and exit")
	pbVersionShort := stFlagSet.Bool("V", false, "show verinc program version and exit (short)")

	// Help / usage flags: wir werten sie über os.Args aus (wie bisher)
	// Parse known flags first
	if err := stFlagSet.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// Aliases handled manually from raw os.Args
	for _, szArg := range os.Args[1:] {
		switch szArg {
		case "-?", "-h", "--help", "--usage", "help", "usage", "-H", "-help", "-U", "-usage":
			stCfg.bShowHelp = true
		}
	}

	// Merge long/short versions
	stCfg.bVerbose = *pbVerbose || *pbVerboseShort
	stCfg.bGet = *pbGet || *pbGetShort
	stCfg.bMinor = *pbMinor || *pbMinorShort
	stCfg.bMajor = *pbMajor || *pbMajorShort
	stCfg.bShowVersion = *pbVersion || *pbVersionShort

	// Remaining arguments after flag parsing are files
	stCfg.aszFiles = stFlagSet.Args()

	if len(stCfg.aszFiles) == 0 && !stCfg.bShowHelp && !stCfg.bShowVersion && !stCfg.bGet {
		return nil, errors.New("no files specified! Try --help for usage information.")
	}

	return stCfg, nil
}

func fnInfo(bVerbose bool, szMsg string, aArgs ...interface{}) {
	if !bVerbose {
		return
	}
	fmt.Fprintf(os.Stdout, "ℹ️ [!] "+szMsg+"\n", aArgs...)
}

func fnError(szMsg string, aArgs ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ [!] "+szMsg+"\n", aArgs...)
}

func fnBumpVersion(szVersion string, bMinor bool, bMajor bool) (string, error) {
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
		iMinor++			// this means we support sth. like 3.42.6 -> 3.43.1
		iPatch = 1		// if you want 3.9.6 -> 3.10.1, you have to use -j/--major
	} else {
		// Patch bump with overflow rule
		iPatch++
		if iPatch >= 10 {
			iPatch = 1
			iMinor++
		}
	}

	szNew := fmt.Sprintf("%d.%d.%d", iMajor, iMinor, iPatch)
	return szNew, nil
}

// FIXED: fnProcessLine now only einmal vorhanden, mit verbose-Ausgabe old->new
func fnProcessLine(szFile string, szLine string, bMinor bool, bMajor bool, bVerbose bool, pbChanged *bool) (string, error) {

	if !strings.Contains(szLine, "_VERSION") &&
		!strings.Contains(szLine, "ProductVersion\":") &&
		!strings.Contains(szLine, "\"Version\":") &&
		!strings.Contains(szLine, "FileVersion\":") {
		return szLine, nil
	}

	aszMatch := reVersionPattern.FindStringSubmatch(szLine)
	if len(aszMatch) != 4 {
		return szLine, fmt.Errorf("no version pattern found: %v (len=%d): %q", aszMatch, len(aszMatch), szLine)
	}

	szOldVersion := aszMatch[0]
	szNewVersion, err := fnBumpVersion(szOldVersion, bMinor, bMajor)
	if err != nil {
		return szLine, err
	}

	szNewLine := strings.Replace(szLine, szOldVersion, szNewVersion, 1)
	*pbChanged = true

	if bVerbose {
		fmt.Fprintf(os.Stdout,
			"ℹ️ [!] verinc: %s: %s -> %s\n",
			szFile, szOldVersion, szNewVersion)
	}

	return szNewLine, nil
}

func fnGetVersion(szFile string) (string, error) {
	fIn, err := os.Open(szFile)
	if err != nil {
		return "", err
	}
	defer fIn.Close()

	stScanner := bufio.NewScanner(fIn)
	for stScanner.Scan() {
		szLine := stScanner.Text()

		// This section will be enhanced in the future to support more patterns, but for now we can just filter lines that don't contain any of the relevant tokens to speed up processing.
		if !strings.Contains(szLine, "_VERSION") &&
			!strings.Contains(szLine, "ProductVersion\"") &&
			!strings.Contains(szLine, "\"Version\"") &&
			!strings.Contains(szLine, "FileVersion\"") {
			continue
		}

		aszMatch := reVersionPattern.FindStringSubmatch(szLine)
		if len(aszMatch) == 4 {
			return aszMatch[0], nil
		}
	}
	if err := stScanner.Err(); err != nil {
		return "", err
	}

	return "", errors.New("no version pattern found")
}

func fnProcessFile(szPath string, stCfg *tConfig) (bool, int, error) {
	fnInfo(stCfg.bVerbose, "Processing file: %s", szPath)

	fIn, err := os.Open(szPath)
	if err != nil {
		return false, iExitFileReadError, err
	}
	defer fIn.Close()

	var aszLines []string
	bAnyChanged := false

	stScanner := bufio.NewScanner(fIn)
	for stScanner.Scan() {
		szLine := stScanner.Text()
		bChanged := false

		szNewLine, err := fnProcessLine(szPath, szLine, stCfg.bMinor, stCfg.bMajor, stCfg.bVerbose, &bChanged)
		if err != nil && stCfg.bVerbose {
			fnInfo(stCfg.bVerbose, "Version processing warning in %s: %v", szPath, err)
		}
		if bChanged {
			bAnyChanged = true
		}
		aszLines = append(aszLines, szNewLine)
	}
	if err := stScanner.Err(); err != nil {
		return false, iExitFileReadError, err
	}

	if !bAnyChanged {
		fnInfo(stCfg.bVerbose, "No _VERSION/ProductVersion/FileVersion line updated in %s", szPath)
		return false, iExitNoVersionLines, nil
	}

	fOut, err := os.Create(szPath)
	if err != nil {
		return false, iExitFileWriteError, err
	}
	defer fOut.Close()

	stWriter := bufio.NewWriter(fOut)
	for _, szLine := range aszLines {
		if _, err := stWriter.WriteString(szLine + "\n"); err != nil {
			return false, iExitFileWriteError, err
		}
	}
	if err := stWriter.Flush(); err != nil {
		return false, iExitFileWriteError, err
	}

	fnInfo(stCfg.bVerbose, "Updated file: %s", szPath)
	return true, iExitOK, nil
}

func main() {
	flag.CommandLine.Usage = fnMainUsage

	stCfg, err := fnParseArgs()
	if err != nil {
		// Fehlertext von errors.New(...) direkt vergleichen ist unzuverlässig;
		// daher einfach generisch behandeln:
		if err.Error() == "no files specified" {
			fnError("No files specified.")
			os.Exit(iExitNoFiles)
		}
		fnError("Invalid command line arguments: %v", err)
		os.Exit(iExitBadArguments)
	}

	if stCfg.bShowHelp {
		fnMainUsage()
		os.Exit(iExitOK)
	}

	if stCfg.bShowVersion {
		fmt.Fprintf(os.Stdout, "verinc version %s\n@(#) $Id: verinc.go,v 1.8 2026/04/17 14:27:46 ralph Exp $\n", szVerincVersion)
		os.Exit(iExitOK)
	}

	if stCfg.bGet {
		// --get mode: return first matching version string to stdout
		if len(stCfg.aszFiles) == 0 {
			fnError("No files specified.")
			os.Exit(iExitNoFiles)
		}

		for _, szPath := range stCfg.aszFiles {
			szVersion, err := fnGetVersion(szPath)
			if err != nil {
				fnError("Error processing file %s: %v", szPath, err)
				os.Exit(iExitNoVersionLines)
			}
			fmt.Fprintf(os.Stdout, "%s\n", szVersion)
		}
		os.Exit(iExitOK)
	}

	if len(stCfg.aszFiles) == 0 {
		fnError("No files specified.")
		os.Exit(iExitNoFiles)
	}

	bAnyFileChanged := false
	iLastExitCode := iExitOK

	for _, szPath := range stCfg.aszFiles {
		bChanged, iCode, err := fnProcessFile(szPath, stCfg)
		if err != nil && iCode != iExitNoVersionLines {
			fnError("Error processing file %s: %v", szPath, err)
			os.Exit(iCode)
		}
		if bChanged {
			bAnyFileChanged = true
		}
		if iCode != iExitOK {
			iLastExitCode = iCode
		}
	}

	if !bAnyFileChanged {
		fnError("No _VERSION/ProductVersion/FileVersion lines updated in any file.")
		os.Exit(iExitNoVersionLines)
	}

	os.Exit(iLastExitCode)
}
