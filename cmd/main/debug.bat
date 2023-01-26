go mod tidy -compat=1.17
go mod download

set /p tags=CGO_ENABLED=0/go:build tags: || set "tags=vweb_lib yaegi_lib igop_lib"
set version=App/%date:~0,4%%date:~5,2%%date:~8,2%

set CGO_ENABLED=0

set GOOS=windows
set GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe -gcflags "-N -l" -ldflags="-X main.version=%version%" -tags="%tags%" ./

set PATH=K:\code\GO\bin;%PATH%
cd /D bin
V-WEB-Server-win-amd64.exe -RootDir ../test -ConfigFile config.json

pause