set GO111MODULE=on


set GOOS=windows
set GOARCH=amd64
go build -o ../bin/V-WEB-Server-win-amd64.exe -ldflags="-s -w" ../main

set GOOS=linux
set GOARCH=amd64
go build -o ../bin/V-WEB-Server-linux-amd64 -ldflags="-s -w" ../main


pause