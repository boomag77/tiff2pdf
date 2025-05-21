package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
		switch args.OutputFileType {
		case "pdf":
			errs = append(errs, fmt.Errorf("output directory is required for PDF conversion"))
		case "tiff":
			if args.TIFFMode == "convert" {
				errs = append(errs, fmt.Errorf("output directory is required for TIFF conversion"))
			}
		}
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
	fileType := strings.ToLower(args.OutputFileType)
	if fileType != "pdf" && fileType != "tiff" {
		errs = append(errs, fmt.Errorf("mode must be either 'pdf' or 'tiff'"))
	}

	tiffMode := strings.ToLower(args.TIFFMode)
	if tiffMode != "replace" && tiffMode != "convert" && tiffMode != "append" {
		errs = append(errs, fmt.Errorf("TIFF mode must be either 'replace', 'convert' or 'append'"))
	}
	if fileType == "tiff" && tiffMode == "" {
		errs = append(errs, fmt.Errorf("TIFF mode is required for TIFF conversion"))
	}

	ccittMode := strings.ToLower(args.CCITT)
	if ccittMode != "on" && ccittMode != "off" && ccittMode != "auto" {
		errs = append(errs, fmt.Errorf("CCITT mode must be either 'on', 'off' or 'auto'"))
	}

	if args.RGBdpi <= 0 {
		errs = append(errs, fmt.Errorf("RGB DPI must be positive"))
	}
	if args.GrayDpi <= 0 {
		errs = append(errs, fmt.Errorf("gray DPI must be positive"))
	}
	if args.RGBJpegQuality < 1 || args.RGBJpegQuality > 100 {
		errs = append(errs, fmt.Errorf("RGB JPEG quality must be between 1 and 100"))
	}
	if args.GrayJpegQuality < 1 || args.GrayJpegQuality > 100 {
		errs = append(errs, fmt.Errorf("gray JPEG quality must be between 1 and 100"))
	}

	if len(errs) == 0 {
		return nil
	}

	return errs
}

func main() {

	inputRootDir := flag.String("input", "", "Input directory containing folders with TIFF files or TIFF files")
	outputs := []string{}
	flag.Func("output", "Output directory for converted files", func(outputDirPath string) error {
		outputs = append(outputs, outputDirPath)
		return nil
	})
	fileType := flag.String("type", "pdf", "Type of the output file: pdf, tiff")

	tiffMode := flag.String("tiffmode", "replace", "TIFF mode: replace, convert, append")

	ccitt_compression := flag.String("ccitt", "off", "CCITT compression: on, off, auto")

	dpiRGB := flag.Int("rgbdpi", 300, "DPI fo RGB images")
	dpiGray := flag.Int("grdpi", 300, "DPI for grayscale images")
	jpegRGBQuality := flag.Int("rgbq", 100, "JPEG quality (1-100) for RGB images")
	jpegGrayQuality := flag.Int("grq", 100, "JPEG quality (1-100) for grayscale images")
	flag.Parse()

	// for testing
	// flag.Visit(func(f *flag.Flag) {
	// 	fmt.Printf("Пойман флаг: %s = %s\n", f.Name, f.Value)
	// })

	params := InputFlags{
		InputRootDir:    *inputRootDir,
		OutputDir:       outputs,
		OutputFileType:  *fileType,
		CCITT:           *ccitt_compression,
		TIFFMode:        *tiffMode,
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

	fmt.Println(string(Yellow), "-------------------", string(Reset))
	if params.OutputFileType == "pdf" {
		switch params.CCITT {
		case "on":
			fmt.Println("TIFFS -> PDF (resampled if needed) with CCITTFAXG4 compression")
		case "off":
			fmt.Println("TIFFS -> PDF (resampled if needed)")
		case "auto":
			fmt.Println("TIFFS -> PDF (resampled if needed) with CCITTFAXG4 compression (if possible)")
		}
	} else {
		switch params.CCITT {
		case "on":
			fmt.Println("TIFFS -> TIFFS (resampled if needed) with CCITTFAXG4 compression")
		case "off":
			fmt.Println("TIFFS -> TIFFS (resampled if needed)")
		case "auto":
			fmt.Println("TIFFS -> TIFFS (resampled if needed) with CCITTFAXG4 compression (if possible)")
		}
		switch params.TIFFMode {
		case "replace":
			fmt.Println("TIFF mode: replace - replace original TIFF files")
		case "convert":
			fmt.Println("TIFF mode: convert - convert original TIFF files to new TIFF files")
		case "append":
			fmt.Println("TIFF mode: append - append converted TIFF files to original TIFF files")
		}
	}
	fmt.Println("INPUT DIR:\n", params.InputRootDir)
	if len(params.OutputDir) > 0 {
		fmt.Println("OUTPUT DIR(s):\n", strings.Join(params.OutputDir, "\n "))
	}
	if params.RGBdpi > 0 {
		fmt.Println("TARGET RGB DPI: ", params.RGBdpi)
	} else {
		fmt.Println("TARGET RGB DPI: Image original")
	}
	if params.GrayDpi > 0 {
		fmt.Println("TARGET GRAY DPI: ", params.GrayDpi)
	} else {
		fmt.Println("TARGET GRAY DPI: Image original")
	}
	if params.CCITT == "off" || params.CCITT == "auto" {
		fmt.Println("TARGET RGB JPEG Quality: ", params.RGBJpegQuality)
		fmt.Println("TARGET GRAY JPEG Quality: ", params.GrayJpegQuality)
	}
	fmt.Println(string(Yellow), "-------------------", string(Reset))

	startTime := time.Now()

	var request ConversionRequest

	if params.OutputFileType == "pdf" {
		tifFldrs, err := files_manager.GetTIFFFolders(params.InputRootDir)
		if err != nil {
			fmt.Printf("Error getting TIFF folders: %v\n", err)
			os.Exit(1)
		}
		if len(tifFldrs) == 0 {
			fmt.Println("No TIFF folders found in the input directory.")
			os.Exit(0)
		}

		request = ConversionRequest{
			Parameters: params,
			Folders:    tifFldrs,
		}
	}

	if params.OutputFileType == "tiff" {
		tiffs, size, err := files_manager.GetTIFFPaths(params.InputRootDir)
		if err != nil {
			fmt.Printf("Error getting TIFF files: %v\n", err)
			os.Exit(1)
		}
		if len(tiffs) == 0 {
			fmt.Println("No TIFF files found in the input directory.")
			os.Exit(0)
		}

		tiffFolder := TIFFfolder{
			TiffFilesPaths: tiffs,
			Name:           filepath.Base(params.InputRootDir),
			Path:           params.InputRootDir,
			TiffFilesSize:  size,
		}

		request = ConversionRequest{
			Parameters: params,
			Folders:    []TIFFfolder{tiffFolder},
		}
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signals
		fmt.Printf("Received signal: %s. Exiting...\n", sig)
		os.Exit(1)
	}()

	if err := converter.Convert(request); err != nil {
		fmt.Printf("Error during conversion: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(Green), "Conversion completed successfully.", string(Reset))
	fmt.Printf("Total time taken: %v\n", time.Since(startTime))
}
