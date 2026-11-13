@echo off
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w -H=windowsgui" -o LinkVideo.Tools.exe .
