go get

set GOOS=windows
set GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe -ldflags="-s -w" ./

set GOOS=linux
set GOARCH=amd64
go build -o bin/V-WEB-Server-linux-amd64 -ldflags="-s -w" ./
set GOARCH=arm
set GOARM=7
go build -o bin/V-WEB-Server-linux-armv7 -ldflags="-s -w" ./
set GOARCH=arm64
go build -o bin/V-WEB-Server-linux-arm64 -ldflags="-s -w" ./

upx -9 ./bin/*

pause