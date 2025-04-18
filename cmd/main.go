// #cgo CFLAGS: -I/path/to/include
// #cgo LDFLAGS: -L/path/to/lib -ltiff2png
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"tiff2pdf/contracts"
	"tiff2pdf/converter"
	"tiff2pdf/files_manager"
	"time"
)

type InputFlags = contracts.InputFlags

func main() {
	inputRootDir := flag.String("input", "", "Input directory containing TIFF files")
	outputDir := flag.String("output", "", "Output directory for converted files")
	jpegQuality := flag.Int("quality", 100, "JPEG quality (1-100)")
	flag.Parse()

	args := InputFlags{
		InputRootDir: *inputRootDir,
		OutputDir:    *outputDir,
		JpegQuality:  *jpegQuality,
	}

	fmt.Println("inputRootDir:", args.InputRootDir)
	fmt.Println("outputDir:", args.OutputDir)

	// tiffDirsMap, _ := files_manager.CreateTIFFDirsMap(args.InputRootDir)
	// if len(tiffDirsMap) == 0 {
	// 	fmt.Println("No TIFF files found in the input directory.")
	// 	os.Exit(0)
	// }
	// fmt.Printf("Found %d TIFF directories.\n", len(tiffDirsMap))

	err := files_manager.CheckProvidedDirs(args.InputRootDir, args.OutputDir)
	if err != nil {
		fmt.Printf("[ERROR]: %v\n", err)
		os.Exit(1)
	}

	subDirs, err := files_manager.GetSubDirs(args.InputRootDir)
	if err != nil {
		fmt.Printf("Error getting subdirectories: %v\n", err)
		os.Exit(1)
	}

	startTime := time.Now()
	defer func() {
		fmt.Printf("Total time taken: %s\n", time.Since(startTime))
	}()

	if len(subDirs) == 0 {
		err := converter.Convert(args.InputRootDir, args.OutputDir, args.JpegQuality)
		if err != nil {
			fmt.Printf("Error during conversion: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Conversion completed successfully.")
		os.Exit(0)
	}

	maxConversions := max(runtime.NumCPU()-1, 1)

	sem := make(chan struct{}, maxConversions)

	var wg sync.WaitGroup

	fmt.Println("Starting conversion...")

	for _, subDir := range subDirs {
		wg.Add(1)
		go func(subDir string) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire a token
			defer func() { <-sem }() // Release the token

			err := converter.Convert(subDir, *outputDir, *jpegQuality)
			if err != nil {
				fmt.Printf("Error during conversion in subdirectory %s: %v\n", subDir, err)
				return
			}
		}(subDir)
	}
	wg.Wait()
	fmt.Println("Conversion completed successfully.")

}

func startDaemon(jpegQuality int) (io.WriteCloser, io.ReadCloser, *exec.Cmd, error) {

	wd, err := os.Getwd()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	var cBin string
	var cmd *exec.Cmd

	cBin = filepath.Join(wd, "bin", "tiff2jpg_daemon")
	quality := fmt.Sprintf("--quality=%d", jpegQuality)
	cmd = exec.Command(cBin, quality)

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("daemon start failed: %w", err)
	}
	fmt.Printf("Daemon started with PID: %d\n", cmd.Process.Pid)
	return stdin, stdout, cmd, nil
}
