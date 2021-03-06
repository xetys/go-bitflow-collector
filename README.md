# go-bitflow-collector
go-bitflow-collector is a Go (Golang) tool for collecting time-series data from various sources in high frequency intervals.
It uses the `github.com/bitflow-stream/go-bitflow` library for generating and providing `bitflow.Sample` instances.
The `bitflow-collector` sub-package provides an executable with the same name.
The data collection and other configuration options can be configured through numerous command line flags.

Run `bitflow-collector --help` for a list of command line flags.

The main source of data is the `/proc` filesystem on the local Linux machine (although data collection should also work on other platforms in general).
Other implemented data sources include the remote API provided by `libvirt` and the `OVSDB` protocol offered by Open vSwitch.

## Installation:
* Install packages: `libvirt-dev libpcap-dev`
* Install git and go (at least version **1.8**).
* Make sure `$GOPATH` is set to some existing directory.
* Get and install this tool:

```shell
go get github.com/bitflow-stream/go-bitflow-collector/bitflow-collector
```

* The binary executable `bitflow-collector` will be compiled to `$GOPATH/bin`.
 * Add that directory to your `$PATH`, or copy the executable to a different location.

## Installation without PCAP and LIBVIRT
To avoid installing these dependencies, follow above instructions, but change the `go get` command to the following:
```shell
go get -tags "nopcap nolibvirt" github.com/bitflow-stream/go-bitflow-collector/bitflow-collector
```
