package main

import (
	"flag"
	"fmt"
	"os"
	"tiff2pdf/converter"
)

func main() {
	inputDir := flag.String("input", "", "Input directory containing TIFF files")
	outputDir := flag.String("output", "", "Output directory for converted files")
	flag.Parse()

	if *inputDir == "" || *outputDir == "" {
		fmt.Println("Please provide both input and output directories.")
		return
	}
	// Check if input directory exists
	if stat, err := os.Stat(*inputDir); err != nil || !stat.IsDir() {
		fmt.Println("ERROR: Input directory does not exist or is not a directory.")
		os.Exit(1)
	}
	if stat, err := os.Stat(*outputDir); err != nil || !stat.IsDir() {
		fmt.Println("ERROR: Output directory does not exist or is not a directory.")
		os.Exit(1)
	}

	fmt.Println("Convering TIFF files from ", *inputDir)
	fmt.Println("Save PDF to ", *outputDir)

	err := converter.Convert(*inputDir, *outputDir)
	if err != nil {
		fmt.Println("ERROR: Conversion failed:", err)
		os.Exit(1)
	}
	fmt.Println("Conversion completed successfully.")
}
