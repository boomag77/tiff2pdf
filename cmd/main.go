package main

import (
	"flag"
	"fmt"
	"os"
	"tiff2pdf/contracts"
	"tiff2pdf/converter"
	"tiff2pdf/files_manager"
)

type InputFlags = contracts.InputFlags

func main() {
	fmt.Println("Converting TIFF files to PDF...")

	inputRootDir := flag.String("input", "", "Input directory containing TIFF files")
	outputDir := flag.String("output", "", "Output directory for converted files")
	jpegQuality := flag.Int("quality", 100, "JPEG quality (1-100)")
	flag.Parse()

	if jpegQuality == nil || *jpegQuality < 1 || *jpegQuality > 100 {
		fmt.Println("JPEG quality must be between 1 and 100")
		os.Exit(1)
	}

	args := InputFlags{
		InputRootDir: *inputRootDir,
		OutputDir:    *outputDir,
		JpegQuality:  *jpegQuality,
	}

	fmt.Println("inputRootDir:", args.InputRootDir)
	fmt.Println("outputDir:", args.OutputDir)

	boxFolder, err := files_manager.ResolveBoxFolder(args.InputRootDir)
	if err != nil {
		fmt.Printf("Error resolving box folder: %v\n", err)
		os.Exit(1)
	}
	boxFolder.OutputFolder = args.OutputDir

	if err := converter.Convert(boxFolder, args.JpegQuality); err != nil {
		fmt.Printf("Error during conversion: %v\n", err)
		os.Exit(1)
	}

}
