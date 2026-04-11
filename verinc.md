# verinc - Version Increment Tool

(c) by Ralph Roth, ROSE SWE


`verinc` is a robust, lightweight command-line utility written in Go (v1.25++) designed to automate the management of version strings within source code. It targets specific lines containing version tokens and applies custom incrementation rules, making it ideal for build pipelines and release management.

>[!Hint]
We use this tool in-house for various projects e.g. for cfg2html or getrpm (both available on github too).

---

## 🚀 Features

* **Smart Versioning**: Automatically handles `MAJOR.MINOR.PATCH` increments.
* **Token-Based Selection**: Processes only lines containing specific markers like `_VERSION`, `ProductVersion`, or `FileVersion`.
* **Overflow Logic**: Implements a custom rule where reaching a PATCH of 10 resets it to 1 and increments the MINOR version.
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
| `-m` | `--minor` | Bump **MINOR** version and reset **PATCH** to 1. |
| `-j` | `--major` | Bump **MAJOR** version and reset **MINOR/PATCH** to 0/1. |

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

## ⌨️ Examples

**Basic patch increment:**
```bash
verinc main.pas
```

**Minor version jump with verbose output:**
```bash
verinc -m -v ~/src/project/version.go
```

**Updating multiple files at once:**
```bash
verinc -v config.json constants.pas README.md
```

---

## 🛠 Installation

Build directly from the source using Go:

```bash
go build -o verinc verinc.go
```

// end of document
