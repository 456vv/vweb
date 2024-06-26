go mod tidy -compat=1.17
go get -u

set /p tags=CGO_ENABLED=0/go:build tags: || set "tags=vweb_lib yaegi_lib igop_lib"
set version=App/%date:~0,4%%date:~5,2%%date:~8,2%

set CGO_ENABLED=0

set GOOS=windows
set GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe -trimpath -ldflags="-s -w -X main.version=%version%" -tags="%tags%" ./
go clean -cache

set GOOS=linux
set GOARCH=amd64
go build -o bin/V-WEB-Server-linux-amd64 -trimpath -ldflags="-s -w -X main.version=%version%" -tags="%tags%" ./
go clean -cache

set GOARCH=arm
set GOARM=7
go build -o bin/V-WEB-Server-linux-armv7 -trimpath -ldflags="-s -w -X main.version=%version%" -tags="%tags%" ./
go clean -cache

set GOARCH=arm64
go build -o bin/V-WEB-Server-linux-arm64 -trimpath -ldflags="-s -w -X main.version=%version%" -tags="%tags%" ./
go clean -cache

set GOARCH=mips
go build -o bin/V-WEB-Server-linux-mips -trimpath -ldflags="-s -w -X main.version=%version%" -tags="%tags%" ./
go clean -cache

set /p tags=CGO_ENABLED=1/go:build tags: || set "tags=vweb_lib yaegi_lib igop_lib sqlite3 duckdb"
if "%tags%" == "exit" goto upx
set CGO_ENABLED=1
go build -o bin/V-WEB-Server-win-amd64-general.exe -trimpath -ldflags="-s -w  -X main.version=$version -extldflags '-static -fpic'"  -tags="%tags%" ./

:upx

upx -9 ./bin/*

pause