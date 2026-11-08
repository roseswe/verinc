# verinc - Version Increment Tool

(c) by Ralph Roth, ROSE SWE

`verinc` is a robust, lightweight command-line utility written in Go (v1.25++) designed to automate the management of version strings within source code. It targets specific lines containing version tokens and applies custom incrementation rules, making it ideal for build pipelines and release management.

>[!NOTE]
> We use this tool in-house for various projects e.g. for cfg2html or getrpm (both available on github too).

---

## 🚀 Features

* **Smart Versioning**: Automatically handles `MAJOR.MINOR.PATCH` increments.
* **Token-Based Selection**: Processes only lines containing specific markers like `_VERSION`, `ProductVersion`, or `FileVersion`.
* **Overflow Logic**: Implements a custom rule where reaching a PATCH of 10 resets it to 1 and increments the MINOR version. Can be disabled with `--nowrap`.
* **Safe Updates**: Files are written atomically. Line endings (LF/CRLF) are preserved, file permissions are kept, and the original file remains untouched if anything goes wrong mid-write.
* **Multi-File Support**: Update multiple source files in a single command.
* **Detailed Feedback**: Verbose mode provides a clear "old -> new" transition log for every change.
* **Standard Library**: Built using only the Go standard library for maximum portability.

---

## 🏷️ Versioning Philosophy & Semantic Support

While `verinc` follows a custom overflow rule (e.g., `3.19.9` → `3.20.1`), it is fundamentally designed to support the structure of **Semantic Versioning (SemVer)**.

### Semantic Versioning Reference
The tool operates on the standard `MAJOR.MINOR.PATCH` format:
* **MAJOR**: Incremented for incompatible changes. Use `-j` or `--major`.
* **MINOR**: Incremented for added functionality. Use `-m` or `--minor`.
* **PATCH**: Incremented for backwards-compatible bug fixes. This is the **default** behavior.

### Overflow Behaviour
By default, when PATCH would reach 10, `verinc` wraps: `3.20.9` → `3.21.1`. If you prefer a plain increment without wrapping (`3.20.9` → `3.20.10`, or `1.7.9` → `1.7.10`), pass `-n` or `--nowrap`. The `-m`/`-j` bumps are unaffected by `--nowrap` and still explicitly reset PATCH to 1.

---

## 📂 Supported File Types

`verinc` is language-agnostic and processes files based on line content rather than extension.

### Common Use Cases:
* **Source Code**:
    * **Pascal/Delphi/Lazarus**: Updates constants or comments marked with `_VERSION`.
    * **Go/C/C++**: Updates string constants or header macros.
* **Windows Resource Files (`.rc`)**: Targets `FILEVERSION` and `PRODUCTVERSION` definitions.
* **Compiler & Build Metadata**:
    * **JSON Configs**: Updates `ProductVersion`, `Version`, or `FileVersion` keys found in `package.json` or manifests.
    * **Project Files**: Updates XML or plain-text metadata used by compilers.

---

## 📖 Usage

```bash
verinc [options] file1 [file2 ...]
```

### Options
| Short | Long | Description |
| :--- | :--- | :--- |
| `-?`, `-h` | `--help` | Show detailed help and documentation. |
| `-V` | `--version` | Show current version of the `verinc` tool. |
| `-v` | `--verbose` | Enable verbose output (shows specific line changes). |
| `-g` | `--get` | Return the first matching version string to stdout (useful for Makefiles/scripts). |
| `-m` | `--minor` | Bump **MINOR** version and reset **PATCH** to 1. |
| `-j` | `--major` | Bump **MAJOR** version and reset **MINOR/PATCH** to 0/1. |
| `-n` | `--nowrap` | Bump **PATCH** without wrapping (`1.7.9` → `1.7.10`). |

### Notes
* Only lines containing a recognised version token (`_VERSION`, `ProductVersion`, `FileVersion`, or `"Version"`) are touched.
* The first `MAJOR.MINOR.PATCH` found on such a line is updated.
* Files are updated atomically: line endings and permissions are preserved, and an interrupted run cannot corrupt your source.
* `-h` and `--help` print the help screen and exit with code `0`.

---

## 🚦 Exit Codes

`verinc` returns specific exit codes for easy integration with CI/CD scripts:

* **`0`**: Success ✅
* **`1`**: Generic error
* **`2`**: Invalid command line arguments
* **`3`**: No files provided
* **`4`**: File open/read error
* **`5`**: No version tokens (`_VERSION`, etc.) found in any file
* **`6`**: Version string parse error
* **`7`**: File write error

---

## ✨ Examples

**Basic patch increment:**
```bash
verinc main.pas
```

**Minor version jump with verbose output:**
```bash
verinc -m -v ~/src/project/version.go
```

**Major version jump:**
```bash
verinc -j constants.pas
```

**Patch increment without overflow wrap (1.7.9 → 1.7.10):**
```bash
verinc -n main.pas
```

**Updating multiple files at once:**
```bash
verinc -v config.json constants.pas README.md
```

**Get first matching version string (for Makefiles/Bash scripts):**
```bash
verinc --get version.go
# Output: 2.05

# Useful in Makefiles or shell scripts:
VER=$(verinc -g main.pas)
echo "Version is $VER"
```
## Internal Note on Release handling

Typical workflow with DCO
 1. bump the version inside verinc.go (editor or 'verinc -set verinc.go 2.2.3')
 2. commit with DCO trailer (make commit signs off for you)

        make commit MSG="bump verinc to 2.2.3"

 3. tag and push (settag enforces Signed-off-by, pushtag is idempotent)

        make settag
        make pushtag

or in one shot, after step 2:

        make release

If you forget step 2 and try make settag directly:

>[!WARNING]

        [!!] HEAD commit has no Signed-off-by -- DCO not satisfied
            Run 'make signoff' or use 'make commit MSG="..."' first.

Then make signoff fixes it in place (only safe if HEAD isn't already tagged — the guard catches that case and tells you exactly what to do).

// end of document