//go:build !unix && !windows && !plan9

package base

import "os"

// js、wasip1 等目标不编译 Unix 信号文件，需要提供可移植的信号变量。
var signalsToIgnore = []os.Signal{os.Interrupt}

var SignalTrace os.Signal = nil
