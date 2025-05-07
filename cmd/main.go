package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tiff2pdf/contracts"
	"tiff2pdf/converter"
	"tiff2pdf/files_manager"
	"time"
)

type InputFlags = contracts.InputFlags
type TIFFfolder = contracts.TIFFfolder
type ConversionRequest = contracts.ConversionRequest

const (
	Red    = "\x1b[31m"
	Green  = "\x1b[32m"
	Yellow = "\x1b[33m"
	Blue   = "\x1b[34m"
	Reset  = "\x1b[0m"
)

func defaultFlags() InputFlags {
	return InputFlags{
		RGBdpi:          300,
		GrayDpi:         300,
		GrayJpegQuality: 100,
		RGBJpegQuality:  100,
	}
}

func validateFlags(args InputFlags) []error {
	var errs []error
	if args.InputRootDir != "" {
		if stat, err := os.Stat(args.InputRootDir); err != nil || !stat.IsDir() {
			dirName := filepath.Base(args.InputRootDir)
			errs = append(errs, fmt.Errorf("incorrect input directory %s: does not exist or is not a directory", strings.ToTitle(dirName)))
		}
	} else {
		errs = append(errs, fmt.Errorf("input directory is required"))
	}

	if len(args.OutputDir) == 0 {
		errs = append(errs, fmt.Errorf("at least one output directory is required"))
	}
	if len(args.OutputDir) > 2 {
		errs = append(errs, fmt.Errorf("only two output directories are allowed"))
	}
	for _, outputDir := range args.OutputDir {
		if outputDir != "" {
			if stat, err := os.Stat(outputDir); err != nil || !stat.IsDir() {
				dirName := filepath.Base(outputDir)
				errs = append(errs, fmt.Errorf("output directory %s does not exist or is not a directory", strings.ToTitle(dirName)))
			}
		} else {
			errs = append(errs, fmt.Errorf("output directory path cannot be empty"))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return errs
}

func main() {
	fmt.Println("Converting TIFF files to PDF...")

	inputRootDir := flag.String("input", "", "Input directory containing TIFF files")
	outputs := []string{}
	flag.Func("output", "Output directory for converted files", func(outputDirPath string) error {
		outputs = append(outputs, outputDirPath)
		return nil
	})
	dpiRGB := flag.Int("dpirgb", 300, "DPI fo RGB images")
	dpiGray := flag.Int("dpigr", 300, "DPI for grayscale images")
	jpegRGBQuality := flag.Int("qrgb", 100, "JPEG quality (1-100) for RGB images")
	jpegGrayQuality := flag.Int("qgr", 100, "JPEG quality (1-100) for grayscale images")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		fmt.Printf("  Пойман флаг: %s = %s\n", f.Name, f.Value)
	})

	params := InputFlags{
		InputRootDir:    *inputRootDir,
		OutputDir:       outputs,
		RGBdpi:          *dpiRGB,
		GrayDpi:         *dpiGray,
		GrayJpegQuality: *jpegGrayQuality,
		RGBJpegQuality:  *jpegRGBQuality,
	}

	if errs := validateFlags(params); errs != nil {
		for _, err := range errs {

			fmt.Fprintf(os.Stderr, "- %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Println(params.GrayJpegQuality)

	if params.RGBdpi == 0 {
		params.RGBdpi = defaultFlags().RGBdpi
	}
	if params.GrayDpi == 0 {
		params.GrayDpi = defaultFlags().GrayDpi
	}
	if params.GrayJpegQuality == 0 {
		params.GrayJpegQuality = defaultFlags().GrayJpegQuality
	}
	if params.RGBJpegQuality == 0 {
		params.RGBJpegQuality = defaultFlags().RGBJpegQuality
	}
	fmt.Println("Provided parameters")
	fmt.Println(string(Yellow), "-------------------", string(Reset))
	fmt.Println("INPUT DIR:\n", params.InputRootDir)
	fmt.Println("OUTPUT DIR(s):\n", strings.Join(params.OutputDir, "\n "))
	if params.RGBdpi >= 0 {
		fmt.Println("TARGET RGB DPI: ", params.RGBdpi)
	} else {
		fmt.Println("TARGET RGB DPI: Image original")
	}
	if params.GrayDpi >= 0 {
		fmt.Println("TARGET GRAY DPI: ", params.GrayDpi)
	} else {
		fmt.Println("TARGET GRAY DPI: Image original")
	}
	fmt.Println("TARGET RGB JPEG Quality: ", params.RGBJpegQuality)
	fmt.Println("TARGET GRAY JPEG Quality: ", params.GrayJpegQuality)
	fmt.Println(string(Yellow), "-------------------", string(Reset))
	fmt.Println("Starting conversion...")

	startTime := time.Now()

	tifFldrs, err := files_manager.GetTIFFFolders(params.InputRootDir)
	if err != nil {
		fmt.Printf("Error getting TIFF folders: %v\n", err)
		os.Exit(1)
	}
	if len(tifFldrs) == 0 {
		fmt.Println("No TIFF folders found in the input directory.")
		os.Exit(0)
	}

	request := ConversionRequest{
		Parameters: params,
		Folders:    tifFldrs,
	}

	if err := converter.Convert(request); err != nil {
		fmt.Printf("Error during conversion: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Conversion completed successfully.")
	fmt.Printf("Total time taken: %v\n", time.Since(startTime))
}
