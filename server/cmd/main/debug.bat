taskkill /T /F /IM V-WEB-Server-win-amd64.exe

go mod tidy
go mod download

set GOOS=windows
set GOARCH=amd64
go build -gcflags "-N -l" -o bin/V-WEB-Server-win-amd64.exe

set PATH=K:\code\GO\bin;%PATH%
cd /D bin
V-WEB-Server-win-amd64.exe -RootDir ../test -ConfigFile config.json

pause