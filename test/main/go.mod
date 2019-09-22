module main

require (
	github.com/456vv/vbody v1.1.2
	github.com/456vv/vcipher v1.0.0
	github.com/456vv/vweb v1.3.6
	github.com/fsnotify/fsnotify v1.4.7
	golang.org/x/sys v0.0.0-00010101000000-000000000000 // indirect
)

replace golang.org/x/sys => github.com/golang/sys v0.0.0-20181206074257-70b957f3b65e

replace github.com/qiniu/qlang => github.com/456vv/qlang v0.0.0-20190917160030-fa0675bbd614

go 1.13
