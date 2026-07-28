# xpc

[![CI](https://github.com/Norgate-AV/xpc/workflows/CI/badge.svg)](https://github.com/Norgate-AV/xpc/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/Norgate-AV/xpc)](https://goreportcard.com/report/github.com/Norgate-AV/xpc)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Release](https://img.shields.io/github/v/release/Norgate-AV/xpc)](https://github.com/Norgate-AV/xpc/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

<div align="center">
  <img src="./assets/img/XPanel.png" alt="Xpanel Conversion Tool logo" width="300"/>
</div>

An erognomic CLI wrapper for the [Crestron][crestron] [Xpanel Conversion Tool][xpanelconversiontool].

[crestron]: https://www.crestron.com/
[xpanelconversiontool]: https://www.crestron.com/Software-Firmware/Software/XPanel-Conversion-Tool/1-01-614

## Installation

### Using Scoop

```bash
scoop bucket add norgateav-crestron https://github.com/Norgate-AV/scoop-norgateav-crestron.git
scoop install xpc
```

### Using Go Install

```bash
go install github.com/Norgate-AV/xpc@latest
```

### Manual Installation

1. Clone the repository:

    ```bash
    git clone https://github.com/Norgate-AV/xpc.git && cd smpc
    ```

2. Build and install the binary:

    ```bash
    make install
    ```

    This will compile the `smpc` binary and place it in your `$GOBIN` or `$GOPATH/bin` directory.

## LICENSE

[MIT](./LICENSE)
