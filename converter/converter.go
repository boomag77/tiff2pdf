package converter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"sort"
	"strings"
	"sync"
	"tiff2pdf/contracts"
	"tiff2pdf/pdf_writer"
	"time"
)

const (
	Red    = "\x1b[31m"
	Green  = "\x1b[32m"
	Yellow = "\x1b[33m"
	Blue   = "\x1b[34m"
	Reset  = "\x1b[0m"
)

type BoxFolder = contracts.BoxFolder
type TIFFfolder = contracts.TIFFfolder
type ConversionRequest = contracts.ConversionRequest
type Converter = contracts.Converter
type ConvertResult = contracts.ConvertResult

type ConversionParameters struct {
	CCITT                 string
	TIFFMode              string
	TargetRGBdpi          int
	TargetGraydpi         int
	TargetRGBjpegQuality  int
	TargetGrayjpegQuality int
	Raw                   bool
}

type convertFolderParam struct {
	convParams ConversionParameters
	tiffFolder TIFFfolder
	outputDirs []string
}

type decodeTiffTask struct {
	filePath   string
	pageNumber int
	resultCh   chan ConvertResult
}

type ConvertedDestination struct {
	pdfFilePath string
	tmpFilePath string
	tmpFile     *os.File
}

type OutputFormat string

const (
	pngFormat OutputFormat = "PNG"
	jpgFormat OutputFormat = "JPG"
)

var imgFormat OutputFormat = jpgFormat

//var jpegQualityC = 100

func convertWorker(taskChan <-chan decodeTiffTask, convCfg ConversionParameters, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range taskChan {

		img, err := ConvertTIFF(task.filePath, convCfg)
		if err != nil {
			fmt.Printf("Error encoding JPEG: %v\n", err)
			continue
		}
		//buf := bytes.NewBuffer(data)
		buf := img.Data

		// mmImgWidth := float64(img.Width) * 25.4 / float64(img.ActualDpi)
		// mmImgHeight := float64(img.Height) * 25.4 / float64(img.ActualDpi)
		//x := 0.0
		//y := 0.0
		task.resultCh <- ConvertResult{
			ImageId:     fmt.Sprintf("img_%d", task.pageNumber),
			ImgBuffer:   buf,
			PixelWidth:  img.Width,
			PixelHeight: img.Height,
			CCITT:       img.CCITT != 0,
			Gray:        img.Gray,
			ImgFormat:   string(imgFormat),
			// drawWidth:   mmImgWidth,
			// drawHeight:  mmImgHeight,
			//x:         x,
			//y:         y,
			PageIndex: task.pageNumber,
		}
	}

}

func processTIFFFolder(cfg convertFolderParam) error {

	tiffMode := cfg.convParams.TIFFMode
	filesCount := len(cfg.tiffFolder.TiffFilesPaths)
	numWorkers := min(runtime.NumCPU(), filesCount)

	var processedFilesCount int = 0

	decodeTiffTaskChan := make(chan decodeTiffTask)
	resultChan := make(chan ConvertResult, numWorkers)

	wg := &sync.WaitGroup{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go convertWorker(decodeTiffTaskChan, cfg.convParams, wg)
	}

	done := make(chan struct{})

	go func() {
		for result := range resultChan {
			origFilePath := cfg.tiffFolder.TiffFilesPaths[result.PageIndex]
			//tiffFileName := filepath.Base(cfg.tiffFolder.TiffFilesPaths[result.pageIndex])
			//fmt.Printf("Processing file %s", tiffFileName)
			fmt.Printf("Processed %d%% \r", processedFilesCount*100/filesCount)
			//filePath := filepath.Join(cfg.outputDirs[0], tiffFileName)
			var compression int
			var targetDPI int
			var grayImage bool

			if result.CCITT {
				compression = CompressionCCITTG4
				targetDPI = cfg.convParams.TargetGraydpi
				grayImage = true
			} else {
				if result.Gray {
					compression = CompressionJPEG
					targetDPI = cfg.convParams.TargetGraydpi
					grayImage = true
				} else {
					compression = CompressionJPEG
					targetDPI = cfg.convParams.TargetRGBdpi
					grayImage = false
				}
			}
			err := saveDataToTIFFFile(
				tiffMode,
				origFilePath,
				cfg.outputDirs,
				result.PixelWidth,
				result.PixelHeight,
				result.ImgBuffer,
				targetDPI,
				compression,
				grayImage,
			)
			if err != nil {
				fmt.Printf("%sError saving processed TIFF file %s: %v%s\n", Red, filepath.Base(origFilePath), err, Reset)
				continue
			}
			processedFilesCount++
		}
		if processedFilesCount == filesCount {
			fmt.Println(string(Green), "All files processed and saved successfully", string(Reset))
		} else {
			fmt.Printf("%sProcessed %d files out of %d%s\n", Red, processedFilesCount, filesCount, Reset)
		}
		close(done)
	}()

	for i, file := range cfg.tiffFolder.TiffFilesPaths {
		task := decodeTiffTask{

			filePath:   file,
			pageNumber: i,
			resultCh:   resultChan,
		}
		decodeTiffTaskChan <- task
	}
	close(decodeTiffTaskChan)

	wg.Wait()
	close(resultChan)
	<-done

	return nil
}

func convertFolderToPDF(cfg convertFolderParam) (err error) {
	var pdfPageCount int = 0
	startTime := time.Now()

	if len(cfg.tiffFolder.TiffFilesPaths) == 0 {
		return fmt.Errorf("no TIFF files found in directory %s", cfg.tiffFolder.Name)
	}

	decodeTiffTaskChan := make(chan decodeTiffTask)
	filesCount := len(cfg.tiffFolder.TiffFilesPaths)
	numWorkers := min(runtime.NumCPU(), filesCount)
	resultChan := make(chan ConvertResult, numWorkers)

	wg := &sync.WaitGroup{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go convertWorker(decodeTiffTaskChan, cfg.convParams, wg)
	}

	dirName := strings.TrimSuffix(cfg.tiffFolder.Name, "-2") // pdf file name = tiff files folder name

	destinations := make([]ConvertedDestination, len(cfg.outputDirs))
	writers := make([]io.Writer, len(cfg.outputDirs))
	for i, outputDir := range cfg.outputDirs {
		destinations[i] = ConvertedDestination{
			tmpFilePath: filepath.Join(outputDir, dirName+".tmp"),
			pdfFilePath: filepath.Join(outputDir, dirName+".pdf"),
		}
		f, err := os.Create(destinations[i].tmpFilePath)
		if err != nil {
			return fmt.Errorf("error creating TMP file at output folder %s: %v", filepath.Base(outputDir), err)
		}
		destinations[i].tmpFile = f
		writers[i] = f
	}

	defer func() {
		if err == nil {
			return
		}
		for _, destination := range destinations {
			if destination.tmpFile != nil {
				destination.tmpFile.Close()
			}
			if removeErr := os.Remove(destination.tmpFilePath); removeErr != nil {
				fmt.Printf("Error removing TMP file %s: %v\n", destination.tmpFilePath, removeErr)
			}
		}
	}()

	multipleWriter := io.MultiWriter(writers...)

	pdfWriter, errNewPDFWriter := pdf_writer.NewPDFWriter(multipleWriter)
	if errNewPDFWriter != nil {
		return fmt.Errorf("error creating PDF writer: %v", errNewPDFWriter)
	}

	results := make([]*ConvertResult, filesCount)
	nextIndex := 0

	done := make(chan struct{})

	go func() {
		for result := range resultChan {

			results[result.PageIndex] = &result

			for nextIndex < len(results) && results[nextIndex] != nil {

				err := pdfWriter.WriteImage(results[nextIndex])
				if err != nil {
					fmt.Printf("Failed writing image to PDF: %v\n", err)
					//os.Exit(1)
				} else {
					pdfPageCount++
				}
				results[nextIndex] = nil
				nextIndex++
			}

		}
		close(done)
	}()

	for i, file := range cfg.tiffFolder.TiffFilesPaths {
		task := decodeTiffTask{
			filePath:   file,
			pageNumber: i,
			resultCh:   resultChan,
		}
		decodeTiffTaskChan <- task
	}
	close(decodeTiffTaskChan)

	wg.Wait()
	close(resultChan)
	<-done

	if err := pdfWriter.Finish(); err != nil {
		return fmt.Errorf("error writing PDF file to output folder: %v", err)
	}

	for _, destination := range destinations {
		if err := destination.tmpFile.Sync(); err != nil {
			return fmt.Errorf("error syncing TMP file to output folder %s: %v", filepath.Base(destination.tmpFilePath), err)
		}
		if err := destination.tmpFile.Close(); err != nil {
			return fmt.Errorf("error closing TMP file to output folder %s: %v", filepath.Base(destination.tmpFilePath), err)
		}
	}

	for _, outputDir := range cfg.outputDirs {
		d, err := os.Open(outputDir)
		if err != nil {
			return fmt.Errorf("error opening output directory %s: %v", filepath.Base(outputDir), err)
		}
		if err := d.Sync(); err != nil {
			return fmt.Errorf("error syncing output directory %s: %v", filepath.Base(outputDir), err)
		}
		d.Close()
	}

	for _, destination := range destinations {
		if err := os.Rename(destination.tmpFilePath, destination.pdfFilePath); err != nil {
			return fmt.Errorf("error renaming TMP file to PDF at output folder %s: %v", filepath.Base(destination.tmpFilePath), err)
		}
	}

	endTime := time.Since(startTime)
	if pdfPageCount != len(cfg.tiffFolder.TiffFilesPaths) {
		fmt.Printf("Warning: %d pages written to PDF file, but %d TIFF files were processed\n", pdfPageCount, len(cfg.tiffFolder.TiffFilesPaths))
	} else {
		fmt.Println("Folder " + dirName + " - " + fmt.Sprint(len(cfg.tiffFolder.TiffFilesPaths)) +
			" files converted to PDF with " + fmt.Sprint(pdfPageCount) + " pages. With time: " + endTime.String())
	}

	return nil
}

func Convert(request ConversionRequest) error {

	foldersCount := len(request.Folders)

	maxConversions := foldersCount

	if foldersCount > 1 {
		sort.SliceStable(request.Folders, func(i, j int) bool {
			return len(request.Folders[i].TiffFilesPaths) > len(request.Folders[j].TiffFilesPaths)
		})
	}

	var wg sync.WaitGroup

	sem := make(chan struct{}, maxConversions)

	fmt.Println("Starting conversion...")

	for _, tiffFolder := range request.Folders {
		wg.Add(1)
		go func(tiffFolder contracts.TIFFfolder) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if request.Parameters.OutputFileType == "pdf" {
				folderParams := convertFolderParam{
					tiffFolder: tiffFolder,
					outputDirs: request.Parameters.OutputDir,

					convParams: ConversionParameters{
						Raw:                   false,
						CCITT:                 request.Parameters.CCITT,
						TargetRGBdpi:          request.Parameters.RGBdpi,
						TargetGraydpi:         request.Parameters.GrayDpi,
						TargetRGBjpegQuality:  request.Parameters.RGBJpegQuality,
						TargetGrayjpegQuality: request.Parameters.GrayJpegQuality,
					},
				}
				err := convertFolderToPDF(folderParams)
				if err != nil {
					fmt.Printf("Error during conversion in subdirectory %s: %v\n", tiffFolder.Name, err)
					return
				}
			} else {
				fmt.Printf("Processing TIFF files in folder %s...\n", tiffFolder.Name)
				fmt.Println("TIFF files count: ", len(tiffFolder.TiffFilesPaths))
				fmt.Println("TIFF files size: ", tiffFolder.TiffFilesSize)
				folderParams := convertFolderParam{
					tiffFolder: tiffFolder,
					outputDirs: request.Parameters.OutputDir,
					convParams: ConversionParameters{
						Raw:           true,
						TargetRGBdpi:  request.Parameters.RGBdpi,
						TargetGraydpi: request.Parameters.GrayDpi,
						CCITT:         request.Parameters.CCITT,
						TIFFMode:      request.Parameters.TIFFMode,
					},
				}
				//fmt.Println(folderParams)
				err := processTIFFFolder(folderParams)
				if err != nil {
					fmt.Printf("Error during conversion in subdirectory %s: %v\n", tiffFolder.Name, err)
				}
				return
			}

		}(tiffFolder)
	}
	wg.Wait()
	return nil
}
