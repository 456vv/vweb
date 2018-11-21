set GO111MODULE=on
set GOOS=windows
set GOARCH=386
go build -o V-WEB-Server-win-386.exe -ldflags="-s -w" ../main
set GOARCH=amd64
go build -o V-WEB-Server-win-amd64.exe -ldflags="-s -w" ../main

pause