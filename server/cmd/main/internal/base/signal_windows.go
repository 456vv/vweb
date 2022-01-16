// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package base

import (
	"os"
	"syscall"
)

var signalsToIgnore = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// SignalTrace is the signal to send to make a Go program
// crash with a stack trace (no such signal in this case).
var SignalTrace os.Signal = nil
