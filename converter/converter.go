package converter

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"tiff2pdf/contracts"
	"time"

	"github.com/phpdave11/gofpdf"
)

type BoxFolder = contracts.BoxFolder
type TIFFfolder = contracts.TIFFfolder

type Page struct {
	width  float64
	height float64
}

type convertResult struct {
	imageId    string
	imgFormat  string
	imgBuffer  *bytes.Reader
	drawWidth  float64
	drawHeight float64
	x, y       float64
	pageIndex  int
}

type convertFolderParam struct {
	tiffFolder   TIFFfolder
	boxConverted string
	outputDir    string
	jpegQuality  int
}

type decodeTiffTask struct {
	filePath   string
	pageNumber int
	resultCh   chan convertResult
}

type savePDFTask struct {
	boxConvertedPath string
	outputPath       string
	pdf              *gofpdf.Fpdf
}

type OutputFormat string

const (
	pngFormat OutputFormat = "PNG"
	jpgFormat OutputFormat = "JPG"
)

var imgFormat OutputFormat = jpgFormat
var jpegQualityC = 100

func convertWorker(taskChan <-chan decodeTiffTask, quality int, wg *sync.WaitGroup) {
	defer wg.Done()

	// dpi := 300

	for task := range taskChan {

		jpgData, width, height, dpi, err := ConvertTIFFtoJPEG(task.filePath, quality)
		if err != nil {
			fmt.Printf("Error encoding JPEG: %v\n", err)
			continue
		}
		buf := bytes.NewReader(jpgData)

		mmImgWidth := float64(width) * 25.4 / float64(dpi)
		mmImgHeight := float64(height) * 25.4 / float64(dpi)
		x := 0.0
		y := 0.0
		task.resultCh <- convertResult{
			imageId:    fmt.Sprintf("img_%d", task.pageNumber),
			imgBuffer:  buf,
			imgFormat:  string(imgFormat),
			drawWidth:  mmImgWidth,
			drawHeight: mmImgHeight,
			x:          x,
			y:          y,
			pageIndex:  task.pageNumber,
		}
	}

}

func convertFolder(params convertFolderParam) error {
	startTime := time.Now()

	if len(params.tiffFolder.TiffFilesPaths) == 0 {
		return fmt.Errorf("No TIFF files found in the input directory")
	}

	decodeTiffTaskChan := make(chan decodeTiffTask)
	filesCount := len(params.tiffFolder.TiffFilesPaths)
	resultChan := make(chan convertResult, filesCount)

	var numWorkers int

	numWorkers = min(runtime.NumCPU(), filesCount)

	wg := &sync.WaitGroup{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go convertWorker(decodeTiffTaskChan, params.jpegQuality, wg)
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm"})
	pdf.SetMargins(0, 0, 0)

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

				orientation := "P"
				pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: result.drawWidth, Ht: result.drawHeight})

				pdf.RegisterImageOptionsReader(
					result.imageId,
					gofpdf.ImageOptions{
						ImageType: result.imgFormat,
						ReadDpi:   false,
					}, result.imgBuffer,
				)
				pdf.ImageOptions(
					result.imageId,
					0,
					0,
					result.drawWidth,
					result.drawHeight,
					false,
					gofpdf.ImageOptions{
						ImageType: result.imgFormat,
						ReadDpi:   false,
					},
					0,
					"",
				)
				delete(resultsBuffer, nextIndex)
				nextIndex++
			}

		}
		close(done)
	}()

	for i, file := range params.tiffFolder.TiffFilesPaths {
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

	dirName := params.tiffFolder.Name
	pdfFilePath := filepath.Join(params.outputDir, dirName+".pdf")
	secPdfFilePath := filepath.Join(params.boxConverted, dirName+".pdf")
	pdfPageCount := pdf.PageCount()

	pdfFile, err := os.Create(pdfFilePath)
	if err != nil {
		return fmt.Errorf("Error creating PDF file: %v", err)
	}
	defer pdfFile.Close()
	convertedPdfFile, err := os.Create(secPdfFilePath)
	if err != nil {
		return fmt.Errorf("Error creating PDF file at Converted: %v", err)
	}
	defer convertedPdfFile.Close()

	bwMain := bufio.NewWriterSize(pdfFile, 8*1024*1024)               // 8MB buffer
	bwConverted := bufio.NewWriterSize(convertedPdfFile, 8*1024*1024) // 8MB buffer

	mw := io.MultiWriter(bwMain, bwConverted)

	if err := pdf.Output(mw); err != nil {
		return fmt.Errorf("Error writing PDF file to output folder: %v", err)
	}
	if err := bwConverted.Flush(); err != nil {
		return fmt.Errorf("Error flushing buffer to Converted: %v", err)
	}

	if err := bwMain.Flush(); err != nil {
		return fmt.Errorf("Error flushing buffer: %v", err)
	}

	var startSyncTime time.Time = time.Now()
	if err := convertedPdfFile.Sync(); err != nil {
		return fmt.Errorf("Error syncing PDF file to Converted: %v", err)
	}
	if err := pdfFile.Sync(); err != nil {
		return fmt.Errorf("Error syncing PDF file to output filder: %v", err)
	}
	fmt.Println("PDF file synced to Converted folder with time: " + time.Since(startSyncTime).String())

	endTime := time.Since(startTime)
	fmt.Println("Folder " + dirName + " - " + fmt.Sprint(len(params.tiffFolder.TiffFilesPaths)) +
		" files converted to PDF with " + fmt.Sprint(pdfPageCount) + " pages. With time: " + endTime.String())
	return nil
}

func Convert(boxFolder *BoxFolder, jpegQuality int) error {

	maxConversions := len(boxFolder.FinalizedFolder)
	sort.SliceStable(boxFolder.FinalizedFolder, func(i, j int) bool {
		return len(boxFolder.FinalizedFolder[i].TiffFilesPaths) > len(boxFolder.FinalizedFolder[j].TiffFilesPaths)
	})

	startTime := time.Now()
	defer func() {
		fmt.Printf("Total time taken: %s\n", time.Since(startTime))
	}()
	var wg sync.WaitGroup

	sem := make(chan struct{}, maxConversions)

	fmt.Println("Starting conversion...")

	for _, tiffFolder := range boxFolder.FinalizedFolder {
		wg.Add(1)
		go func(tiffFolder contracts.TIFFfolder) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()
			folderParams := convertFolderParam{
				tiffFolder:   tiffFolder,
				boxConverted: boxFolder.ConvertedFolder.Path,
				outputDir:    boxFolder.OutputFolder,
				jpegQuality:  jpegQuality,
			}
			err := convertFolder(folderParams)
			if err != nil {
				fmt.Printf("Error during conversion in subdirectory %s: %v\n", tiffFolder.Name, err)
				return
			}
		}(tiffFolder)
	}
	wg.Wait()
	fmt.Println("Conversion completed successfully")
	return nil
}
