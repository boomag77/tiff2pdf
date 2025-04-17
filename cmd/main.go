package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"tiff2pdf/converter"
	"time"
)

func main() {
	inputRootDir := flag.String("input", "", "Input directory containing TIFF files")
	outputDir := flag.String("output", "", "Output directory for converted files")
	flag.Parse()

	if *inputRootDir == "" || *outputDir == "" {
		fmt.Println("Please provide both input and output directories.")
		return
	}
	// Check if input directory exists
	if stat, err := os.Stat(*inputRootDir); err != nil || !stat.IsDir() {
		fmt.Println("ERROR: Input directory does not exist or is not a directory.")
		os.Exit(1)
	}
	if stat, err := os.Stat(*outputDir); err != nil || !stat.IsDir() {
		fmt.Println("ERROR: Output directory does not exist or is not a directory.")
		os.Exit(1)
	}

	subDirs, err := getSubDirs(*inputRootDir)
	if err != nil {
		fmt.Printf("Error getting subdirectories: %v\n", err)
		os.Exit(1)
	}

	startTime := time.Now()
	defer func() {
		fmt.Printf("Total time taken: %s\n", time.Since(startTime))
	}()

	if len(subDirs) == 0 {
		err := converter.Convert(*inputRootDir, *outputDir)
		if err != nil {
			fmt.Printf("Error during conversion: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Conversion completed successfully.")
		return
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

			err := converter.Convert(subDir, *outputDir)
			if err != nil {
				fmt.Printf("Error during conversion in subdirectory %s: %v\n", subDir, err)
				return
			}
		}(subDir)
	}
	wg.Wait()
	fmt.Println("Conversion completed successfully.")

}

func getSubDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var subDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			subDirPath := filepath.Join(root, entry.Name())
			subDirs = append(subDirs, subDirPath)
		}
	}
	return subDirs, nil
}
