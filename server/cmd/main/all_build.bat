go mod tidy
go mod download

set CGO_ENABLED=0
set GOOS=linux
set GOARCH=386
go build -o bin/V-WEB-Server-linux-386 -ldflags="-s -w" ./
set GOARCH=amd64
go build -o bin/V-WEB-Server-linux-amd64 -ldflags="-s -w" ./
set GOARCH=arm
set GOARM=7
go build -o bin/V-WEB-Server-linux-armv7 -ldflags="-s -w" ./
set GOARCH=arm64
go build -o bin/V-WEB-Server-linux-arm64 -ldflags="-s -w" ./
set GOARCH=mips
go build -o bin/V-WEB-Server-linux-mips -ldflags="-s -w" ./


set GOOS=windows
set GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe -ldflags="-s -w" ./

::set CGO_ENABLED=1
::go build -o bin/V-WEB-Server-win-amd64-general.exe -ldflags="-s -w -extldflags '-static -fpic'" -tags "general" ./

upx -9 ./bin/*

pause