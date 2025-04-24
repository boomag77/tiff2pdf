package converter

import (
	"bufio"
	"bytes"
	"fmt"
	"image/jpeg"

	//"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"tiff2pdf/contracts"
	"time"

	//"github.com/chai2010/tiff"
	"github.com/phpdave11/gofpdf"
	//"golang.org/x/image/tiff"
	exiftiff "github.com/rwcarlsen/goexif/tiff"
)

type BoxFolder = contracts.BoxFolder
type TIFFfolder = contracts.TIFFfolder

// type Page struct {
// 	width  float64
// 	height float64
// }

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

func convertWorker(taskChan <-chan decodeTiffTask, quality int, wg *sync.WaitGroup) {
	defer wg.Done()

	// dpi := 300

	for task := range taskChan {

		//jpgData, width, height, dpi, err := ConvertTIFFtoJPEG(task.filePath, quality)
		jpgData, width, height, dpi, err := ConvertTIFFtoJPEGExp(task.filePath, quality)
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
		return fmt.Errorf("no TIFF files found in the input directory")
	}

	decodeTiffTaskChan := make(chan decodeTiffTask)
	filesCount := len(params.tiffFolder.TiffFilesPaths)
	resultChan := make(chan convertResult, filesCount)

	numWorkers := min(runtime.NumCPU(), filesCount)

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
		return fmt.Errorf("error creating PDF file: %v", err)
	}
	defer pdfFile.Close()
	convertedPdfFile, err := os.Create(secPdfFilePath)
	if err != nil {
		return fmt.Errorf("error creating PDF file at Converted: %v", err)
	}
	defer convertedPdfFile.Close()

	bwMain := bufio.NewWriterSize(pdfFile, 8*1024*1024)               // 8MB buffer
	bwConverted := bufio.NewWriterSize(convertedPdfFile, 8*1024*1024) // 8MB buffer

	mw := io.MultiWriter(bwMain, bwConverted)

	if err := pdf.Output(mw); err != nil {
		return fmt.Errorf("error writing PDF file to output folder: %v", err)
	}
	if err := bwConverted.Flush(); err != nil {
		return fmt.Errorf("error flushing buffer to Converted: %v", err)
	}

	if err := bwMain.Flush(); err != nil {
		return fmt.Errorf("error flushing buffer: %v", err)
	}

	var startSyncTime time.Time = time.Now()
	if err := convertedPdfFile.Sync(); err != nil {
		return fmt.Errorf("error syncing PDF file to Converted: %v", err)
	}
	if err := pdfFile.Sync(); err != nil {
		return fmt.Errorf("error syncing PDF file to output filder: %v", err)
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

// func ConvertTIFFtoJPEG(path string, quality int) (jpegData []byte, width, height, dpi int, err error) {
// 	// Open the TIFF file
// 	file, err := os.Open(path)
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to open TIFF file: %w", err)
// 	}
// 	defer file.Close()

// 	// Get file stats to seek back to beginning later
// 	// stat, err := file.Stat()
// 	// if err != nil {
// 	// 	return nil, 0, 0, 0, fmt.Errorf("failed to get file stats: %w", err)
// 	// }

// 	// Try to extract DPI and other metadata first
// 	dpi, err = extractDPIFromTIFF(file)
// 	if err != nil {
// 		// If we can't get DPI, use default
// 		dpi = 300
// 	}

// 	// Seek back to beginning of file
// 	_, err = file.Seek(0, io.SeekStart)
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to seek file: %w", err)
// 	}

// 	// Decode the TIFF
// 	img, err := tiff.Decode(file)
// 	if err != nil {
// 		// If standard decoder fails, try reading as an LZW compressed TIFF
// 		_, err = file.Seek(0, io.SeekStart)
// 		if err != nil {
// 			return nil, 0, 0, 0, fmt.Errorf("failed to seek file: %w", err)
// 		}

// 		// You might need a custom decoder for LZW here
// 		// This is where you'd implement specialized LZW handling
// 	}

// 	if img == nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to decode TIFF image")
// 	}

// 	// Get image dimensions
// 	bounds := img.Bounds()
// 	width = bounds.Dx()
// 	height = bounds.Dy()

// 	// Create a buffer for the JPEG output
// 	var buf bytes.Buffer

// 	// Create JPEG encoder options
// 	options := jpeg.Options{
// 		Quality: quality,
// 	}

// 	// Encode the image as JPEG
// 	err = jpeg.Encode(&buf, img, &options)
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to encode JPEG: %w", err)
// 	}

// 	// Return the JPEG data
// 	return buf.Bytes(), width, height, dpi, nil
// }

// // Extract DPI from TIFF
// func extractDPIFromTIFF(r io.ReadSeeker) (int, error) {
// 	// Implementation depends on the TIFF library you use
// 	// This is a placeholder - you'll need to implement based on your library
// 	return 300, nil
// }

//"github.com/disintegration/imaging"
// func ConvertTIFFtoJPEG(path string, quality int) (jpegData []byte, width, height, dpi int, err error) {
// 	// Open and decode the TIFF file using imaging library
// 	img, err := imaging.Open(path)
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to open/decode TIFF: %w", err)
// 	}

// 	// Get image dimensions
// 	bounds := img.Bounds()
// 	width = bounds.Dx()
// 	height = bounds.Dy()

// 	// Default DPI
// 	dpi = 300

// 	// Create a buffer for the JPEG output
// 	var buf bytes.Buffer

// 	// Create JPEG encoder options
// 	options := jpeg.Options{
// 		Quality: quality,
// 	}

// 	// Encode the image as JPEG
// 	err = jpeg.Encode(&buf, img, &options)
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to encode JPEG: %w", err)
// 	}

// 	// Return the JPEG data
// 	return buf.Bytes(), width, height, dpi, nil
// }

//tiff "github.com/chai2010/tiff"
// func ConvertTIFFtoJPEGExp(path string, quality int) (jpegData []byte, width, height, dpi int, err error) {
// 	// 1) Читаем весь файл
// 	data, err := os.ReadFile(path)
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("read file: %w", err)
// 	}

// 	// 2) Декодируем все страницы (если мультистраничный TIFF) в [][]image.Image
// 	pages, _, err := tiff.DecodeAll(bytes.NewReader(data))
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("decode tiff: %w", err)
// 	}
// 	if len(pages) == 0 || len(pages[0]) == 0 {
// 		return nil, 0, 0, 0, fmt.Errorf("no images in tiff")
// 	}

// 	img := pages[0][0] // первая страница
// 	bounds := img.Bounds()
// 	width, height = bounds.Dx(), bounds.Dy()
// 	dpi = 300 // по умолчанию

// 	// 3) Кодируем в JPEG
// 	buf := &bytes.Buffer{}
// 	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality}); err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("encode jpeg: %w", err)
// 	}

// 	return buf.Bytes(), width, height, dpi, nil
// }

func ConvertTIFFtoJPEGExp(path string, quality int) (jpegData []byte, width, height, dpi int, err error) {
	// Открываем файл
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	// Парсим TIFF в структуру Tiff
	tf, err := exiftiff.Decode(f)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("decode tiff: %w", err)
	}
	if len(tf.Dirs) == 0 {
		return nil, 0, 0, 0, fmt.Errorf("no IFD in tiff")
	}
	dir := tf.Dirs[0]

	// Собираем параметры Old-Style JPEG
	var (
		ileOff, ileLen int
		tables         []byte
		offs, cnts     []int
	)
	for _, tag := range dir.Tags {
		switch tag.Id {
		case 0x0201: // JPEGInterchangeFormat
			if v, e := tag.Int(0); e == nil {
				ileOff = v
			}
		case 0x0202: // JPEGInterchangeFormatLength
			if v, e := tag.Int(0); e == nil {
				ileLen = v
			}
		case 0x0157: // JPEGTables
			tables = tag.Val
		case 273: // StripOffsets
			for i := 0; i < int(tag.Count); i++ {
				if v, e := tag.Int(i); e == nil {
					offs = append(offs, v)
				}
			}
		case 279: // StripByteCounts
			for i := 0; i < int(tag.Count); i++ {
				if v, e := tag.Int(i); e == nil {
					cnts = append(cnts, v)
				}
			}
		}
	}

	var rawJPEG []byte
	if ileOff > 0 && ileLen > 0 {
		// Вариант 1: единый JPEG-блок
		rawJPEG = make([]byte, ileLen)
		if _, err := f.ReadAt(rawJPEG, int64(ileOff)); err != nil && err != io.EOF {
			return nil, 0, 0, 0, fmt.Errorf("read JPEGInterchange: %w", err)
		}
	} else {
		// Вариант 2: strip-блоки + таблицы
		if len(offs) == 0 || len(cnts) == 0 {
			return nil, 0, 0, 0, fmt.Errorf("not a JPEG-TIFF (missing strips)")
		}
		buf := &bytes.Buffer{}
		buf.Write([]byte{0xFF, 0xD8}) // SOI

		// Вставляем таблицы без своих SOI/EOI
		if len(tables) >= 4 && tables[0] == 0xFF && tables[1] == 0xD8 {
			tables = tables[2 : len(tables)-2]
		}
		buf.Write(tables)

		// Читаем и пишем подряд все strip-чанки
		for i, off := range offs {
			chunk := make([]byte, cnts[i])
			if _, err := f.ReadAt(chunk, int64(off)); err != nil && err != io.EOF {
				return nil, 0, 0, 0, fmt.Errorf("read strip %d: %w", i, err)
			}
			buf.Write(chunk)
		}
		buf.Write([]byte{0xFF, 0xD9}) // EOI
		rawJPEG = buf.Bytes()
	}

	// Декодируем JPEG, узнаём размеры
	img, err := jpeg.Decode(bytes.NewReader(rawJPEG))
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("jpeg decode: %w", err)
	}
	b := img.Bounds()
	width, height, dpi = b.Dx(), b.Dy(), 300

	// Перекодируем под нужное качество
	out := &bytes.Buffer{}
	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, 0, fmt.Errorf("jpeg encode: %w", err)
	}
	return out.Bytes(), width, height, dpi, nil
}
