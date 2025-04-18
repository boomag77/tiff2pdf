package converter

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"tiff2pdf/utils"

	"github.com/phpdave11/gofpdf"
	//"gopkg.in/gographics/imagick.v2/imagick"
	//"github.com/davidbyttow/govips/v2/vips"
)

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

type convertTask struct {
	filePath   string
	pageNumber int
	resultCh   chan convertResult
}

type OutputFormat string

const (
	pngFormat OutputFormat = "PNG"
	jpgFormat OutputFormat = "JPG"
)

var imgFormat OutputFormat = jpgFormat
var jpegQualityC = 100
var pngCompressionLevel = png.DefaultCompression

func GetTiffFiles(inputDir string) ([]string, int64, error) {

	var size int64 = 0
	var tiffFilesCount int = 0

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, 0, err
	}

	files := make([]string, 0, len(entries))

	//var dirs = make(map[string]int)

	err = filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// if _, exists := dirs[path]; !exists {
			// 	dirs[path] = 0
			// }
			return nil
		}

		if strings.HasPrefix(d.Name(), "._") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".tiff" || ext == ".tif" {
			//dirs[filepath.Dir(path)]++
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
			files = append(files, path)
			tiffFilesCount++
		}
		return nil
	})
	if err != nil {
		return nil, size, fmt.Errorf("Error while scanning directory: %v", err)
	}
	return files, size, nil
}

func decodeTIFF(filePath string) ([]byte, error) {

	if daemonStdin != nil && daemonStdout != nil {
		return decodeFromDaemon(filePath)
	}
	return decodeWithC(filePath)
}

func decodeWithC(tiffPath string) ([]byte, error) {

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var cBin string
	var cmd *exec.Cmd
	if imgFormat == pngFormat {
		cBin = filepath.Join(wd, "bin", "tiff2png")
		cmd = exec.Command(cBin, tiffPath)
	} else {
		cBin = filepath.Join(wd, "bin", "tiff2jpg")
		quality := fmt.Sprintf("--quality=%d", jpegQualityC)
		cmd = exec.Command(cBin, tiffPath, quality)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	if err != nil {
		return nil, fmt.Errorf("tiff2png failed: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func getTIFFName(filePath string) string {
	return strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
}

func convertWorker(taskChan <-chan convertTask, daemon *DecoderDaemon, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range taskChan {

		if imgFormat == pngFormat {
			pngData, err := decodeTIFF(task.filePath)
			if err != nil {
				fmt.Printf("Error decoding image: %v\n", err)
				return
			}

			buf := bytes.NewReader(pngData)

			cfg, _, err := image.DecodeConfig(bytes.NewReader(pngData))
			if err != nil {
				fmt.Printf("Error decoding PNG config: %v\n", err)
				return
			}

			dpi, err := utils.GetDPIfromPNG(pngData)
			if err != nil {
				dpi = 72
			}

			mmImgWidth := float64(cfg.Width) * 25.4 / dpi
			mmImgHeight := float64(cfg.Height) * 25.4 / dpi

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
		} else {
			jpgData, err := daemon.Decode(task.filePath)
			if err != nil {
				fmt.Printf("Error decoding image: %v\n", err)
				return
			}
			buf := bytes.NewReader(jpgData)
			cfg, _, err := image.DecodeConfig(bytes.NewReader(jpgData))
			if err != nil {
				fmt.Printf("Error decoding JPG config: %v\n", err)
				return
			}
			dpi := 300.0
			mmImgWidth := float64(cfg.Width) * 25.4 / dpi
			mmImgHeight := float64(cfg.Height) * 25.4 / dpi
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

func Convert(inputDir string, outputDir string, jpegQuality int) error {

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

	numWorkers := runtime.NumCPU()

	wg := &sync.WaitGroup{}

	daemons, err := StartDaemonPool(numWorkers, jpegQuality)
	if err != nil {
		return fmt.Errorf("Error starting daemon pool: %v", err)
	}
	defer func() {
		for _, daemon := range daemons {
			_ = daemon.Stdin.Close()
			_ = daemon.Cmd.Wait()
		}
	}()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go convertWorker(taskChan, daemons[i], wg)
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
	pdfPageCount := pdf.PageCount()
	err = pdf.OutputFileAndClose(pdfFilePath)
	if err != nil {
		return fmt.Errorf("Error saving PDF file: %v", err)
	}

	//fmt.Printf("Converted to %s\n", pdfFilePath)
	fmt.Println("Folder " + dirName + " - " + fmt.Sprint(len(filesToConvert)) + " files converted to PDF with " + fmt.Sprint(pdfPageCount) + " pages")
	return nil
}
