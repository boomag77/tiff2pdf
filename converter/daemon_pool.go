package converter

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type DecoderDaemon struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Cmd    *exec.Cmd
}

func startDaemon(jpegQuality int) (*DecoderDaemon, error) {

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cBin := filepath.Join(wd, "bin", "tiff2jpg_daemon")
	quality := fmt.Sprintf("--quality=%d", jpegQuality)
	cmd := exec.Command(cBin, quality)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	go io.Copy(io.Discard, stderr)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("daemon start failed: %w", err)
	}
	//fmt.Printf("Daemon started with PID: %d\n", cmd.Process.Pid)
	return &DecoderDaemon{
		Stdin:  stdin,
		Stdout: stdout,
		Cmd:    cmd,
	}, nil
}

func StartDaemonPool(n int, jpegQuality int) ([]*DecoderDaemon, error) {
	pool := make([]*DecoderDaemon, n)
	for i := 0; i < n; i++ {
		d, err := startDaemon(jpegQuality)
		if err != nil {
			return nil, fmt.Errorf("failed to start daemon %d: %w", i, err)
		}
		pool[i] = d
	}
	return pool, nil
}

func (d *DecoderDaemon) Decode(filePath string) ([]byte, error) {

	_, err := fmt.Fprintln(d.Stdin, filePath)
	if err != nil {
		return nil, fmt.Errorf("write to daemon stdin: %w", err)
	}

	var sizeBuf [4]byte
	if _, err := io.ReadFull(d.Stdout, sizeBuf[:]); err != nil {
		return nil, fmt.Errorf("read size: %w", err)
	}
	size := binary.LittleEndian.Uint32(sizeBuf[:])

	buf := make([]byte, size)
	if _, err := io.ReadFull(d.Stdout, buf); err != nil {
		return nil, fmt.Errorf("read jpeg: %w", err)
	}
	return buf, nil
}
