package contracts

import "io"

type DaemonImpl struct {
	StdIn io.ReadCloser
	StdOut io.WriteCloser
}

