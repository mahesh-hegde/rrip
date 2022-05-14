#!/bin/sh

GOOS=linux GOARCH=amd64 go build
tar -cJf rrip_linux_amd64.tar.xz rrip
rm rrip

GOOS=windows GOARCH=amd64 go build
zip -r rrip_windows64.zip rrip.exe
rm rrip.exe

GOOS=darwin GOARCH=amd64 go build
tar -cJf rrip_macos_intel.tar.xz rrip
rm rrip

GOOS=darwin GOARCH=arm64 go build
tar -cJf rrip_macos_arm64.tar.xz rrip
rm rrip

