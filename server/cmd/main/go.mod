module main

go 1.14

require (
	github.com/456vv/vbody latest
	github.com/456vv/vcipher latest
	github.com/456vv/vconn latest
	github.com/456vv/vconnpool latest
	github.com/456vv/verifycode latest
	github.com/456vv/verror latest
	github.com/456vv/vforward latest
	github.com/456vv/vmap/v2 latest
	github.com/456vv/vweb/v2 v2.0.0-00010101000000-000000000000
	github.com/456vv/vweb/v2/builtin v0.0.0-00010101000000-000000000000
	github.com/456vv/vweb/v2/server v0.0.0-00010101000000-000000000000
	github.com/456vv/vweb/v2/server/watch v0.0.0-00010101000000-000000000000
	github.com/fsnotify/fsnotify latest
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/mattn/anko v0.1.8
	golang.org/x/image latest
)

replace github.com/456vv/vweb/v2 => ../../../

replace github.com/456vv/vweb/v2/builtin => ../../../builtin

replace github.com/456vv/vweb/v2/server => ../../../server

replace github.com/456vv/vweb/v2/server/watch => ../../../server/watch
