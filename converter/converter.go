package converter

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"tiff2pdf/contracts"
	"tiff2pdf/utils"
	"time"

	"github.com/phpdave11/gofpdf"
	//"gopkg.in/gographics/imagick.v2/imagick"
	//"github.com/davidbyttow/govips/v2/vips"
)

type BoxFolder = contracts.BoxFolder

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
	tiffFolder   contracts.TIFFfolder
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

var pdfSaveTasks chan savePDFTask
var pdfSaveWG sync.WaitGroup

var imgFormat OutputFormat = jpgFormat
var jpegQualityC = 100
var pngCompressionLevel = png.DefaultCompression

func Init() {
	pdfSaveTasks = make(chan savePDFTask, 100)
	startPDFSaverWorkers(pdfSaveTasks, &pdfSaveWG, 2)
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

func convertWorker(taskChan <-chan decodeTiffTask, daemon *DecoderDaemon, wg *sync.WaitGroup) {
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
			//startTime := time.Now()
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
			//fmt.Printf("Decoded %s in %s\n", task.filePath, time.Since(startTime))
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

func convertFolder(params convertFolderParam) error {
	if len(params.tiffFolder.TiffFilesPaths) == 0 {
		return fmt.Errorf("No TIFF files found in the input directory")
	}

	decodeTiffTaskChan := make(chan decodeTiffTask)
	bufferSize := calcBufferSize(params.tiffFolder.TiffFilesSize)
	resultChan := make(chan convertResult, bufferSize)

	numWorkers := runtime.NumCPU()

	wg := &sync.WaitGroup{}

	daemons, err := StartDaemonPool(numWorkers, params.jpegQuality)
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
		go convertWorker(decodeTiffTaskChan, daemons[i], wg)
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

	//dirName := filepath.Base(filepath.Clean(inputDir))
	dirName := params.tiffFolder.Name
	pdfFilePath := filepath.Join(params.outputDir, dirName+".pdf")
	pdfCopyPath := filepath.Join(params.boxConverted, dirName+".pdf")
	pdfPageCount := pdf.PageCount()

	pdfSaveWG.Add(1)
	pdfSaveTasks <- savePDFTask{
		pdf:              pdf,
		boxConvertedPath: pdfCopyPath,
		outputPath:       pdfFilePath,
	}
	//pdfSaveWG.Wait()
	//err = pdf.OutputFileAndClose(pdfFilePath)
	// if err != nil {
	// 	return fmt.Errorf("Error saving PDF file: %v", err)
	// }

	//fmt.Printf("Converted to %s\n", pdfFilePath)
	fmt.Println("Folder " + dirName + " - " + fmt.Sprint(len(params.tiffFolder.TiffFilesPaths)) +
		" files converted to PDF with " + fmt.Sprint(pdfPageCount) + " pages")
	return nil
}

func startPDFSaverWorkers(taskChan <-chan savePDFTask, wg *sync.WaitGroup, workers int) {
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			for task := range taskChan {
				if err := task.pdf.OutputFileAndClose(task.outputPath); err != nil {
					fmt.Printf("[Worker %d] Error saving PDF file: %v\n", workerID, err)
					return
				}
				if err := copyFile(task.outputPath, task.boxConvertedPath); err != nil {
					fmt.Printf("[Worker %d] Error copying PDF file: %v\n", workerID, err)
					return
				}
				wg.Done()
			}
		}(i)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func Convert(boxFolder *BoxFolder, jpegQuality int) error {

	Init()
	defer func() {
		close(pdfSaveTasks)
		pdfSaveWG.Wait()

	}()

	startTime := time.Now()
	defer func() {
		fmt.Printf("Total time taken: %s\n", time.Since(startTime))
	}()

	maxConversions := max(runtime.NumCPU()-1, 1)

	sem := make(chan struct{}, maxConversions)

	var wg sync.WaitGroup

	fmt.Println("Starting conversion...")

	for _, tiffFolder := range boxFolder.FinalizedFolder {
		wg.Add(1)
		go func(tiffFolder contracts.TIFFfolder) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire a token
			defer func() { <-sem }() // Release the token
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
