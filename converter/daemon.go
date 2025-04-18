package converter

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

var (
	daemonStdin  io.WriteCloser
	daemonStdout io.ReadCloser
	daemonMutex  sync.Mutex
)

func SetDaemonIO(stdIn io.WriteCloser, stdOut io.ReadCloser) {
	daemonStdin = stdIn
	daemonStdout = stdOut
}

func decodeFromDaemon(filePath string) ([]byte, error) {
	daemonMutex.Lock()
	defer daemonMutex.Unlock()

	if _, err := fmt.Fprintln(daemonStdin, filePath); err != nil {
		return nil, fmt.Errorf("write to daemon stdin: %w", err)
	}

	var sizeBuf [4]byte
	if _, err := io.ReadFull(daemonStdout, sizeBuf[:]); err != nil {
		return nil, fmt.Errorf("read jpeg length: %w", err)
	}
	size := binary.LittleEndian.Uint32(sizeBuf[:])

	jpegBuf := make([]byte, size)
	if _, err := io.ReadFull(daemonStdout, jpegBuf); err != nil {
		return nil, fmt.Errorf("read jpeg data: %w", err)
	}
	return jpegBuf, nil
}
