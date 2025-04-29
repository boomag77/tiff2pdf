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
	dpi := flag.Int("dpi", 300, "DPI")
	jpegQuality := flag.Int("quality", 100, "JPEG quality (1-100)")
	flag.Parse()

	if *dpi < 72 || *dpi > 300 {
		*dpi = 300
	}

	if *jpegQuality < 1 || *jpegQuality > 100 {
		*jpegQuality = 100
	}

	args := InputFlags{
		InputRootDir: *inputRootDir,
		OutputDir:    *outputDir,
		Dpi:          *dpi,
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

	if err := converter.Convert(boxFolder, args.JpegQuality, args.Dpi); err != nil {
		fmt.Printf("Error during conversion: %v\n", err)
		os.Exit(1)
	}

}
