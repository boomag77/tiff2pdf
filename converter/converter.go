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

type BoxFolder = contracts.BoxFolder
type TIFFfolder = contracts.TIFFfolder
type ConversionRequest = contracts.ConversionRequest

type convertResult struct {
	imgBuffer []byte
	imageId   string
	imgFormat string
	//imgBuffer   io.Reader
	pixelWidth  int
	pixelHeight int
	drawWidth   float64
	drawHeight  float64
	x, y        float64
	pageIndex   int
	gray        bool
	ccitt       bool
}

type ConversionParameters struct {
	targetRGBdpi          int
	targetGraydpi         int
	targetRGBjpegQuality  int
	targetGrayjpegQuality int
}

type convertFolderParam struct {
	convParams ConversionParameters
	tiffFolder TIFFfolder
	outputDirs []string
}

type decodeTiffTask struct {
	filePath   string
	pageNumber int
	resultCh   chan convertResult
}

type ConvertedDestination struct {
	pdfFilePath string
	tmpFilePath string
	tmpFile     *os.File
}

// type savePDFTask struct {
// 	boxConvertedPath string
// 	outputPath       string
// 	pdf              *gofpdf.Fpdf
// }

type OutputFormat string

const (
	pngFormat OutputFormat = "PNG"
	jpgFormat OutputFormat = "JPG"
)

var imgFormat OutputFormat = jpgFormat

//var jpegQualityC = 100

func convertWorker(taskChan <-chan decodeTiffTask, convCfg ConversionParameters, wg *sync.WaitGroup) {
	defer wg.Done()

	// dpi := 300

	for task := range taskChan {

		img, err := ConvertTIFF(task.filePath, convCfg)
		if err != nil {
			fmt.Printf("Error encoding JPEG: %v\n", err)
			continue
		}
		//buf := bytes.NewBuffer(data)
		buf := img.Data

		mmImgWidth := float64(img.Width) * 25.4 / float64(img.ActualDpi)
		mmImgHeight := float64(img.Height) * 25.4 / float64(img.ActualDpi)
		x := 0.0
		y := 0.0
		task.resultCh <- convertResult{
			imageId:     fmt.Sprintf("img_%d", task.pageNumber),
			imgBuffer:   buf,
			pixelWidth:  img.Width,
			pixelHeight: img.Height,
			ccitt:       img.CCITT != 0,
			gray:        img.Gray,
			imgFormat:   string(imgFormat),
			drawWidth:   mmImgWidth,
			drawHeight:  mmImgHeight,
			x:           x,
			y:           y,
			pageIndex:   task.pageNumber,
		}
	}

}

func convertFolder(cfg convertFolderParam) (err error) {
	var pdfPageCount int = 0
	startTime := time.Now()

	if len(cfg.tiffFolder.TiffFilesPaths) == 0 {
		return fmt.Errorf("no TIFF files found in directory %s", cfg.tiffFolder.Name)
	}

	decodeTiffTaskChan := make(chan decodeTiffTask)
	filesCount := len(cfg.tiffFolder.TiffFilesPaths)
	resultChan := make(chan convertResult, filesCount)

	numWorkers := min(runtime.NumCPU(), filesCount)

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

	pdfWriter, _ := pdf_writer.NewPDFWriter(multipleWriter)
	// TODO : add error handling for pdfWriter

	resultsBuffer := make(map[int]convertResult, filesCount)
	nextIndex := 0

	done := make(chan struct{})

	go func() {
		for result := range resultChan {

			resultsBuffer[result.pageIndex] = result

			for {

				result, ok := resultsBuffer[nextIndex]
				if !ok {
					break
				}

				if result.ccitt {
					if err := pdfWriter.WriteCCITTImage(
						result.pixelWidth,
						result.pixelHeight,
						result.imgBuffer,
					); err == nil {
						pdfPageCount++
					} else {
						fmt.Printf("Error writing CCITT image: %v\n", err)
					}
				} else {
					if result.gray {
						if err := pdfWriter.WriteGrayJPEGImage(
							result.pixelWidth,
							result.pixelHeight,
							result.imgBuffer,
						); err == nil {
							pdfPageCount++
						} else {
							fmt.Printf("Error writing grayscale image: %v\n", err)
						}
					} else {
						if err := pdfWriter.WriteRGBJPEGImage(
							result.pixelWidth,
							result.pixelHeight,
							result.imgBuffer,
						); err == nil {
							pdfPageCount++
						} else {
							fmt.Printf("Error writing RGB image: %v\n", err)
						}
					}
				}

				delete(resultsBuffer, nextIndex)
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
	fmt.Println("Folder " + dirName + " - " + fmt.Sprint(len(cfg.tiffFolder.TiffFilesPaths)) +
		" files converted to PDF with " + fmt.Sprint(pdfPageCount) + " pages. With time: " + endTime.String())
	return nil
}

func Convert(request ConversionRequest) error {

	maxConversions := len(request.Folders)

	sort.SliceStable(request.Folders, func(i, j int) bool {
		return len(request.Folders[i].TiffFilesPaths) > len(request.Folders[j].TiffFilesPaths)
	})

	var wg sync.WaitGroup

	sem := make(chan struct{}, maxConversions)

	fmt.Println("Starting conversion...")

	for _, tiffFolder := range request.Folders {
		wg.Add(1)
		go func(tiffFolder contracts.TIFFfolder) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()
			folderParams := convertFolderParam{
				tiffFolder: tiffFolder,
				outputDirs: request.Parameters.OutputDir,

				convParams: ConversionParameters{
					targetRGBdpi:          request.Parameters.RGBdpi,
					targetGraydpi:         request.Parameters.GrayDpi,
					targetRGBjpegQuality:  request.Parameters.RGBJpegQuality,
					targetGrayjpegQuality: request.Parameters.GrayJpegQuality,
				},
			}
			err := convertFolder(folderParams)
			if err != nil {
				fmt.Printf("Error during conversion in subdirectory %s: %v\n", tiffFolder.Name, err)
				return
			}
		}(tiffFolder)
	}
	wg.Wait()
	return nil
}

// func convertFolder(params convertFolderParam) error {
// 	startTime := time.Now()

// 	if len(params.tiffFolder.TiffFilesPaths) == 0 {
// 		return fmt.Errorf("no TIFF files found in the input directory")
// 	}

// 	decodeTiffTaskChan := make(chan decodeTiffTask)
// 	filesCount := len(params.tiffFolder.TiffFilesPaths)
// 	resultChan := make(chan convertResult, filesCount)

// 	numWorkers := min(runtime.NumCPU(), filesCount)

// 	wg := &sync.WaitGroup{}

// 	for i := 0; i < numWorkers; i++ {
// 		wg.Add(1)
// 		go convertWorker(decodeTiffTaskChan, params.jpegQuality, params.dpi, params.scale, wg)
// 	}

// 	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm"})
// 	pdf.SetMargins(0, 0, 0)

// 	resultsBuffer := make(map[int]convertResult, filesCount)
// 	nextIndex := 0

// 	done := make(chan struct{})

// 	go func() {
// 		for result := range resultChan {

// 			resultsBuffer[result.pageIndex] = result

// 			for {

// 				result, ok := resultsBuffer[nextIndex]
// 				if !ok {
// 					break
// 				}

// 				orientation := "P"
// 				pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: result.drawWidth, Ht: result.drawHeight})

// 				pdf.RegisterImageOptionsReader(
// 					result.imageId,
// 					gofpdf.ImageOptions{
// 						ImageType: result.imgFormat,
// 						ReadDpi:   false,
// 					}, result.imgBuffer,
// 				)
// 				pdf.ImageOptions(
// 					result.imageId,
// 					0,
// 					0,
// 					result.drawWidth,
// 					result.drawHeight,
// 					false,
// 					gofpdf.ImageOptions{
// 						ImageType: result.imgFormat,
// 						ReadDpi:   false,
// 					},
// 					0,
// 					"",
// 				)
// 				delete(resultsBuffer, nextIndex)
// 				nextIndex++
// 			}

// 		}
// 		close(done)
// 	}()

// 	for i, file := range params.tiffFolder.TiffFilesPaths {
// 		task := decodeTiffTask{
// 			filePath:   file,
// 			pageNumber: i,
// 			resultCh:   resultChan,
// 		}
// 		decodeTiffTaskChan <- task
// 	}
// 	close(decodeTiffTaskChan)

// 	wg.Wait()
// 	close(resultChan)
// 	<-done

// 	dirName := params.tiffFolder.Name
// 	pdfFilePath := filepath.Join(params.outputDir, dirName+".pdf")
// 	secPdfFilePath := filepath.Join(params.boxConverted, dirName+".pdf")
// 	pdfPageCount := pdf.PageCount()

// 	pdfFile, err := os.Create(pdfFilePath)
// 	if err != nil {
// 		return fmt.Errorf("error creating PDF file: %v", err)
// 	}
// 	defer pdfFile.Close()
// 	convertedPdfFile, err := os.Create(secPdfFilePath)
// 	if err != nil {
// 		return fmt.Errorf("error creating PDF file at Converted: %v", err)
// 	}
// 	defer convertedPdfFile.Close()

// 	bwMain := bufio.NewWriterSize(pdfFile, 8*1024*1024)               // 8MB buffer
// 	bwConverted := bufio.NewWriterSize(convertedPdfFile, 8*1024*1024) // 8MB buffer

// 	mw := io.MultiWriter(bwMain, bwConverted)

// 	if err := pdf.Output(mw); err != nil {
// 		return fmt.Errorf("error writing PDF file to output folder: %v", err)
// 	}
// 	if err := bwConverted.Flush(); err != nil {
// 		return fmt.Errorf("error flushing buffer to Converted: %v", err)
// 	}

// 	if err := bwMain.Flush(); err != nil {
// 		return fmt.Errorf("error flushing buffer: %v", err)
// 	}

// 	var startSyncTime time.Time = time.Now()
// 	if err := convertedPdfFile.Sync(); err != nil {
// 		return fmt.Errorf("error syncing PDF file to Converted: %v", err)
// 	}
// 	if err := pdfFile.Sync(); err != nil {
// 		return fmt.Errorf("error syncing PDF file to output filder: %v", err)
// 	}
// 	fmt.Println("PDF file synced to Converted folder with time: " + time.Since(startSyncTime).String())

// 	endTime := time.Since(startTime)
// 	fmt.Println("Folder " + dirName + " - " + fmt.Sprint(len(params.tiffFolder.TiffFilesPaths)) +
// 		" files converted to PDF with " + fmt.Sprint(pdfPageCount) + " pages. With time: " + endTime.String())
// 	return nil
// }
