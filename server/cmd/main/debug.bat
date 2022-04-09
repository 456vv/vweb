taskkill /T /F /IM V-WEB-Server-win-amd64.exe

go mod download
go mod tidy -compat=1.17

set version=App/%date:~0,4%%date:~5,2%%date:~8,2%
set GOOS=windows
set GOARCH=amd64
go build -gcflags "-N -l" -ldflags="-X main.version=%version%" -tags="vweb_lib yaegi_lib gossa_lib" -o bin/V-WEB-Server-win-amd64.exe

set PATH=K:\code\GO\bin;%PATH%
cd /D bin
V-WEB-Server-win-amd64.exe -RootDir ../test -ConfigFile config.json

pause