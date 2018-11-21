set GO111MODULE=on

set GOOS=windows
set GOARCH=386
go build -o V-WEB-Server-win-386.exe -ldflags="-s -w" ../main
set GOARCH=amd64
go build -o V-WEB-Server-win-amd64.exe -ldflags="-s -w" ../main

set GOOS=linux
set GOARCH=amd64
go build -o V-WEB-Server-linux-amd64 -ldflags="-s -w" ../main
set GOARCH=386
go build -o V-WEB-Server-linux-386 -ldflags="-s -w" ../main
set GOARCH=arm
go build -o V-WEB-Server-linux-arm -ldflags="-s -w" ../main
set GOARCH=mips
go build -o V-WEB-Server-linux-mips -ldflags="-s -w" ../main
set GOARCH=mipsle
go build -o V-WEB-Server-linux-mipsle -ldflags="-s -w" ../main

pause