go get -u

set GO111MODULE=on
set GOPROXY=https://mirrors.aliyun.com/goproxy/,https://goproxy.cn,https://gocenter.io,https://proxy.golang.org,https://goproxy.io,https://athens.azurefd.net,direct
set GOSUMDB=off

set GOOS=windows
set GOARCH=amd64
go build -o V-WEB-Server-win-amd64.exe -ldflags="-s -w" ./

set GOOS=linux
set GOARCH=amd64
go build -o V-WEB-Server-linux-amd64 -ldflags="-s -w" ./


pause