go mod tidy
go mod download

export CGO_ENABLED=0
export version="App/`date +%Y%m%d%H%M%S`"

export GOOS=linux
export GOARCH=386
go build -o bin/V-WEB-Server-linux-386 -ldflags="-s -w -X main.version=$version" -tags="vweb_lib yaegi_lib" ./
export GOARCH=amd64
go build -o bin/V-WEB-Server-linux-amd64 -ldflags="-s -w -X main.version=$version" -tags="vweb_lib yaegi_lib" ./
export GOARCH=arm
export GOARM=6
go build -o bin/V-WEB-Server-linux-armv6 -ldflags="-s -w -X main.version=$version" -tags="vweb_lib yaegi_lib" ./
export GOARCH=arm64
go build -o bin/V-WEB-Server-linux-arm64 -ldflags="-s -w -X main.version=$version" -tags="vweb_lib yaegi_lib" ./
export GOARCH=mips
go build -o bin/V-WEB-Server-linux-mips -ldflags="-s -w -X main.version=$version" -tags="vweb_lib yaegi_lib" ./

export GOOS=windows
export GOARCH=amd64
go build -o bin/V-WEB-Server-win-amd64.exe -ldflags="-s -w -X main.version=$version" -tags="vweb_lib yaegi_lib" ./

#export CGO_ENABLED=1
#
#export PATH=/root/x86_64-linux-musl-cross/bin:$PATH
#export LD_LIBRARY_PATH=/root/x86_64-linux-musl-cross/x86_64-linux-musl/lib:/usr/lib64:/usr/lib:$LD_LIBRARY_PATH
#export LD=x86_64-linux-musl-ld
#export CC=x86_64-linux-musl-gcc
#export CXX=x86_64-linux-musl-g++
#export CPP=x86_64-linux-musl-cpp
#export GOOS=linux
#export GOARCH=amd64
#go build -o bin/V-WEB-Server-linux-amd64-general -ldflags '-s -w -X main.version=$version --extldflags "-static -fpic"' -tags "yaegi_lib libsass" ./
#
#
#export PATH=/root/armv6-linux-musleabihf-cross/bin:$PATH
#export LD_LIBRARY_PATH=/root/armv6-linux-musleabihf-cross/armv6-linux-musleabihf/lib:/usr/lib64:/usr/lib
#export LD=armv6-linux-musleabihf-ld
#export CC=armv6-linux-musleabihf-gcc
#export CXX=armv6-linux-musleabihf-g++
#export CPP=armv6-linux-musleabihf-cpp
#export GOARCH=arm
#export GOARM=6
#go build -o bin/V-WEB-Server-linux-armv6-general -ldflags '-s -w  -X main.version=$version --extldflags "-static -fpic"' -tags "yaegi_lib libsass" ./
#
#upx -9 ./bin/*

pause