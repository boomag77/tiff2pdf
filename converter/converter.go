package converter

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/tiff"
)

type Page struct {
	width  float64
	height float64
}

type convertResult struct {
	imageId    string
	imgBuffer  *bytes.Buffer
	drawWidth  float64
	drawHeight float64
	x, y       float64
	pageIndex  int
}

type convertTask struct {
	filePath   string
	pageNumber int
	resultCh   chan convertResult
}

func (p *Page) calcPageScale(imgWidth float64, imgHeight float64) float64 {
	return p.width / imgWidth
}

func GetTiffFiles(inputDir string) ([]string, int64, error) {
	var files []string
	var size int64 = 0
	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext == ".tiff" || ext == ".tif" {
			size += info.Size()
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, size, fmt.Errorf("Error while scanning directory: %v", err)
	}
	return files, size, nil
}

func getImageFromTiff(filePath string) (image.Image, error) {
	tiffFile, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("Error opening TIFF file: %v", err)
	}
	defer tiffFile.Close()
	img, err := tiff.Decode(tiffFile)
	if err != nil {
		return nil, fmt.Errorf("Error decoding TIFF file: %v", err)
	}
	return img, nil
}

func getTIFFName(filePath string) string {
	return strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
}

func convertWorker(taskChan <-chan convertTask, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range taskChan {
		img, err := getImageFromTiff(task.filePath)
		if err != nil {
			fmt.Printf("Error getting image from file %s: %v\n", task.filePath, err)
			return
		}
		var buf bytes.Buffer
		png.Encode(&buf, img)
		bounds := img.Bounds()

		mmImgWidth := float64(bounds.Dx()) * 25.4
		mmImgHeight := float64(bounds.Dy()) * 25.4

		x := 0.0
		y := 0.0

		task.resultCh <- convertResult{
			imageId:    fmt.Sprintf("img_%d", task.pageNumber),
			imgBuffer:  &buf,
			drawWidth:  mmImgWidth,
			drawHeight: mmImgHeight,
			x:          x,
			y:          y,
			pageIndex:  task.pageNumber,
		}
	}
}

func calcBufferSize(totalSize int64) int {
	const maxBufferSize = 200 * 1024 * 1024 // 200 MB
	approxPNGBytes := int64(float64(totalSize) * 1)
	estimatedPagesInMemory := int(maxBufferSize / approxPNGBytes)
	if estimatedPagesInMemory < 1 {
		estimatedPagesInMemory = 1
	}
	return estimatedPagesInMemory
}

func Convert(inputDir string, outputDir string) error {

	filesToConvert, totalSize, err := GetTiffFiles(inputDir)
	if err != nil {
		return fmt.Errorf("Error getting TIFF files: %v", err)
	}
	if len(filesToConvert) == 0 {
		return fmt.Errorf("No TIFF files found in the input directory")
	}

	taskChan := make(chan convertTask)
	bufferSize := calcBufferSize(totalSize)
	resultChan := make(chan convertResult, bufferSize)

	numWorkers := 4

	wg := &sync.WaitGroup{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go convertWorker(taskChan, wg)
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm"})
	pdf.SetMargins(0, 0, 0)

	resultsBuffer := make(map[int]convertResult)
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
						ImageType: "PNG",
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
						ImageType: "PNG",
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

	for i, file := range filesToConvert {
		task := convertTask{
			filePath:   file,
			pageNumber: i,
			resultCh:   resultChan,
		}
		taskChan <- task
	}
	close(taskChan)

	wg.Wait()
	close(resultChan)
	<-done

	dirName := filepath.Base(filepath.Clean(inputDir))
	pdfFilePath := filepath.Join(outputDir, dirName+".pdf")
	err = pdf.OutputFileAndClose(pdfFilePath)
	if err != nil {
		return fmt.Errorf("Error saving PDF file: %v", err)
	}

	fmt.Printf("Converted to %s\n", pdfFilePath)
	return nil
}
