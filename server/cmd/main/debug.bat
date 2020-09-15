set GOOS=windows
set GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe

set PATH=K:\code\GO\bin;%PATH%
gdlv exec bin/V-WEB-Server-win-amd64.exe -RootDir ./test -ConfigFile config.json
