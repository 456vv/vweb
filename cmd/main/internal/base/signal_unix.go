// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build aix || darwin || dragonfly || freebsd || js || linux || netbsd || openbsd || solaris
// +build aix darwin dragonfly freebsd js linux netbsd openbsd solaris

package base

import (
	"os"
	"syscall"
)

var signalsToIgnore = []os.Signal{syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2}

// SignalTrace is the signal to send to make a Go program
// crash with a stack trace.
var SignalTrace os.Signal = syscall.SIGQUIT
