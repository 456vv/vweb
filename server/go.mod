module github.com/456vv/vweb/v2/server

go 1.14

require (
	github.com/456vv/vbody v1.2.2
	github.com/456vv/vcipher v1.0.0
	github.com/456vv/vconn v1.0.3
	github.com/456vv/vconnpool v1.1.2
	github.com/456vv/verifycode v1.0.3
	github.com/456vv/verror v1.1.0
	github.com/456vv/vforward v1.0.3
	github.com/456vv/vmap/v2 v2.2.6
	github.com/456vv/vweb/v2 v2.0.0-00010101000000-000000000000
	github.com/456vv/vweb/v2/builtin v0.0.0-00010101000000-000000000000
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	golang.org/x/image v0.0.0-20200618115811-c13761719519 // indirect
)

replace github.com/456vv/vweb/v2 => ../
replace github.com/456vv/vweb/v2/builtin => ../builtin
