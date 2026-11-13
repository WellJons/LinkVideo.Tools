@echo off
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-s -w -H=windowsgui" -o LinkVideo.ServerTools.exe .
