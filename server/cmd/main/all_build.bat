go mod tidy
go mod download

set version=App/%date:~0,4%%date:~5,2%%date:~8,2%%time:~0,2%%time:~3,2%%time:~6,2%
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=386
go build -o bin/V-WEB-Server-linux-386 -ldflags="-s -w -X main.version=%version%" ./
set GOARCH=amd64
go build -o bin/V-WEB-Server-linux-amd64 -ldflags="-s -w -X main.version=%version%" ./
set GOARCH=arm
set GOARM=7
go build -o bin/V-WEB-Server-linux-armv7 -ldflags="-s -w -X main.version=%version%" ./
set GOARCH=arm64
go build -o bin/V-WEB-Server-linux-arm64 -ldflags="-s -w -X main.version=%version%" ./
set GOARCH=mips
go build -o bin/V-WEB-Server-linux-mips -ldflags="-s -w -X main.version=%version%" ./


set GOOS=windows
set GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe -ldflags="-s -w -X main.version=%version%" ./

::set CGO_ENABLED=1
::go build -o bin/V-WEB-Server-win-amd64-general.exe -ldflags="-s -w  -X main.version=$version -extldflags '-static -fpic'" -tags "general" ./

upx -9 ./bin/*

pause