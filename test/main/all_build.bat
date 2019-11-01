go get -u

set GO111MODULE=on
set GOPROXY=https://mirrors.aliyun.com/goproxy/,https://goproxy.cn,https://gocenter.io,https://proxy.golang.org,https://goproxy.io,https://athens.azurefd.net,direct
set GOSUMDB=off

set GOOS=windows
set GOARCH=386
go build -o V-WEB-Server-win-386.exe -ldflags="-s -w" ./
set GOARCH=amd64
go build -o V-WEB-Server-win-amd64.exe -ldflags="-s -w" ./

set GOOS=linux
set GOARCH=amd64
go build -o V-WEB-Server-linux-amd64 -ldflags="-s -w" ./
set GOARCH=386
go build -o V-WEB-Server-linux-386 -ldflags="-s -w" ./
set GOARCH=arm
go build -o V-WEB-Server-linux-arm -ldflags="-s -w" ./
set GOARCH=arm64
go build -o V-WEB-Server-linux-arm64 -ldflags="-s -w" ./
set GOARCH=mips
go build -o V-WEB-Server-linux-mips -ldflags="-s -w" ./


pause