// Package buildinfo - версія збірки. CI підставляє коміт:
//
//	go build -ldflags "-X kvstore/internal/buildinfo.Version=<коміт>"
package buildinfo

var Version = "dev"
