//go:build cgo
// +build cgo

package converter

/*

#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -ljpeg -ltiff



#include <stdlib.h>
#include <tiffio.h>

#include "settings.h"
#include "converter.h"


*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

const (
	// TIFF compression types
	CompressionNone    = C.COMPRESSION_NONE
	CompressionCCITTG4 = C.COMPRESSION_CCITTFAX4
	CompressionJPEG    = C.COMPRESSION_JPEG
	CompressionLZW     = C.COMPRESSION_LZW
)

type ImageData struct {
	Data      []byte
	CCITT     int
	Gray      bool
	Width     int
	Height    int
	ActualDpi int
}

func ConvertTIFF(path string, convParams ConversionParameters) (ImageData, error) {

	cPath := C.CString(path) // Converts Go string to C string
	defer func() {
		if cPath != nil {
			C.free(unsafe.Pointer(cPath))
		}
	}()

	var outBuf *C.uchar
	var outSize C.ulong
	var w, h C.size_t
	var d C.int
	var use_ccitt C.int
	var use_gray C.bool

	switch convParams.CCITT {
	case "auto":
		use_ccitt = 0
	case "on":
		use_ccitt = 1
	case "off":
		use_ccitt = -1
	}

	comp := C.get_compression_type(cPath)
	if comp == 2 || comp == 3 || comp == 4 {
		// CCITT
		ccitt := 1
		rc := C.extract_ccitt_raw(
			cPath,
			&outBuf, &outSize,
			&w, &h)
		if rc != 0 {
			C.free(unsafe.Pointer(outBuf))
			return ImageData{}, fmt.Errorf("ExtractCCITTRaw failed with code %d", int(rc))
		}
		dataSize := int(outSize)
		data := C.GoBytes(unsafe.Pointer(outBuf), C.int(dataSize))
		if outBuf != nil {
			C.free(unsafe.Pointer(outBuf))
		}
		return ImageData{
			Data:   data,
			CCITT:  ccitt,
			Gray:   false,
			Width:  int(w),
			Height: int(h),
		}, nil
	}

	rawFlag := 0

	if convParams.Raw {
		rawFlag = 1
	}

	options := C.tiff_convert_options{
		raw:             C.int(rawFlag),
		path:            cPath,
		rgb_quality:     C.int(convParams.TargetRGBjpegQuality),
		gray_quality:    C.int(convParams.TargetGrayjpegQuality),
		rgb_target_dpi:  C.int(convParams.TargetRGBdpi),
		gray_target_dpi: C.int(convParams.TargetGraydpi),
	}

	rc := C.convert_tiff_to_data(
		&options,
		&outBuf, &outSize,
		&use_ccitt,
		&use_gray,
		&w, &h, &d,
	)
	if rc != 0 {
		if outBuf != nil {
			C.free(unsafe.Pointer(outBuf))
		}
		return ImageData{}, fmt.Errorf("convert_tiff_to_data failed with code %d", int(rc))
	}

	if use_ccitt == 1 && convParams.CCITT != "off" {
		dataSize := int(outSize)
		goGray := C.GoBytes(unsafe.Pointer(outBuf), C.int(dataSize))

		// filtered := medianFilterLight(goGray, int(w), int(h))
		// filtered := medianFilterLight(goGray, int(w), int(h))
		//packed := packGrayTo1BitDither(goGray, int(w), int(h))
		packed := packGrayTo1BitOtsuClose(goGray, int(w), int(h))

		C.free(unsafe.Pointer(outBuf)) // Освобождаем оригинальный буфер

		ccittData, encodeErr := encodeRawCCITTG4(packed, int(w), int(h))
		if encodeErr != nil {
			return ImageData{}, fmt.Errorf("ccittg4 encode failed: %v", encodeErr)
		}

		return ImageData{
			Data:      ccittData,
			CCITT:     int(use_ccitt),
			Gray:      bool(use_gray),
			Width:     int(w),
			Height:    int(h),
			ActualDpi: convParams.TargetGraydpi,
		}, nil
	}
	dataSize := int(outSize)
	data := C.GoBytes(unsafe.Pointer(outBuf), C.int(dataSize))

	if outBuf != nil {
		C.free(unsafe.Pointer(outBuf))
	}

	var actDPI int = 0
	if use_gray {
		actDPI = convParams.TargetGraydpi
	} else {
		actDPI = convParams.TargetRGBdpi
	}

	return ImageData{
		Data:      data,
		CCITT:     int(use_ccitt),
		Gray:      bool(use_gray),
		Width:     int(w),
		Height:    int(h),
		ActualDpi: actDPI,
	}, nil
}

func saveDataToTIFFFile(tiffMode string, origFilePath string, outputs []string, width, height int, data []byte, dpi int, compression int, gray bool) error {

	//fmt.Println("saveDataToTIFFFile: filePath:", filePath, "width:", width, "height:", height, "dpi:", dpi)

	// switch tiffMode {
	// case "convert":
	// case "replace":
	// case "append":
	// default:
	// 	return fmt.Errorf("unsupported tiffMode: %s", tiffMode)
	// }

	if tiffMode == "convert" {
		base := filepath.Base(origFilePath)
		fileName := strings.TrimSuffix(base, filepath.Ext(base))
		tmpProcessedFileName := fileName + ".tmp"
		processedFileName := fileName + ".tif"

		cBuf := C.CBytes(data)
		defer C.free(cBuf)
		grayInt := 0
		if gray {
			grayInt = 1
		}

		var wg sync.WaitGroup
		errs := make(chan error, len(outputs))

		for _, outDir := range outputs {
			outDir := outDir // Capture to local variable
			wg.Add(1)
			go func() {
				defer wg.Done()
				tmpProcessedFilePath := filepath.Join(outDir, tmpProcessedFileName)
				cTmp := C.CString(tmpProcessedFilePath)
				defer C.free(unsafe.Pointer(cTmp))

				C.write_tiff(
					cTmp,
					C.uint32_t(width),
					C.uint32_t(height),
					(*C.uchar)(cBuf),
					C.size_t(len(data)),
					C.int(dpi),
					C.int(compression),
					C.int(grayInt),
				)
				info, err := os.Stat(tmpProcessedFilePath)
				if err != nil {
					errs <- fmt.Errorf("failed to get file info: %v", err)
					return
				}
				if info.Size() == 0 {
					errs <- fmt.Errorf("file is empty: %s", tmpProcessedFilePath)
					return
				}
				if err := os.Rename(tmpProcessedFilePath, filepath.Join(outDir, processedFileName)); err != nil {
					errs <- fmt.Errorf("failed to rename file: %v", err)
					return
				}
				errs <- nil
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return fmt.Errorf("error saving TIFF file: %v", err)
			}
		}
	} else if tiffMode == "replace" {
		base := filepath.Base(origFilePath)
		fileName := strings.TrimSuffix(base, filepath.Ext(base))
		tmpProcessedFileName := fileName + ".tmp"
		processedFileName := fileName + ".tif"
		tmpProcessedFilePath := filepath.Join(filepath.Dir(origFilePath), tmpProcessedFileName)
		processedFilePath := filepath.Join(filepath.Dir(origFilePath), processedFileName)
		cBuf := C.CBytes(data)
		defer C.free(cBuf)
		grayInt := 0
		if gray {
			grayInt = 1
		}
		cTmp := C.CString(tmpProcessedFilePath)
		defer C.free(unsafe.Pointer(cTmp))
		C.write_tiff(
			cTmp,
			C.uint32_t(width),
			C.uint32_t(height),
			(*C.uchar)(cBuf),
			C.size_t(len(data)),
			C.int(dpi),
			C.int(compression),
			C.int(grayInt),
		)
		info, err := os.Stat(tmpProcessedFilePath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %v", err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("file is empty: %s", tmpProcessedFilePath)
		}
		_ = os.Remove(origFilePath)
		if err := os.Rename(tmpProcessedFilePath, processedFilePath); err != nil {
			return fmt.Errorf("failed to rename file: %v", err)
		}
	} else if tiffMode == "append" {
		base := filepath.Base(origFilePath)
		fileName := strings.TrimSuffix(base, filepath.Ext(base))
		tmpProcessedFileName := fileName + ".tmp"
		processedFileName := "_" + fileName + ".tif"
		tmpProcessedFilePath := filepath.Join(filepath.Dir(origFilePath), tmpProcessedFileName)
		processedFilePath := filepath.Join(filepath.Dir(origFilePath), processedFileName)
		cBuf := C.CBytes(data)
		defer C.free(cBuf)
		grayInt := 0
		if gray {
			grayInt = 1
		}
		cTmp := C.CString(tmpProcessedFilePath)
		defer C.free(unsafe.Pointer(cTmp))
		C.write_tiff(
			cTmp,
			C.uint32_t(width),
			C.uint32_t(height),
			(*C.uchar)(cBuf),
			C.size_t(len(data)),
			C.int(dpi),
			C.int(compression),
			C.int(grayInt),
		)
		info, err := os.Stat(tmpProcessedFilePath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %v", err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("file is empty: %s", tmpProcessedFilePath)
		}
		if err := os.Rename(tmpProcessedFilePath, processedFilePath); err != nil {
			return fmt.Errorf("failed to rename file: %v", err)
		}
	} else {
		return fmt.Errorf("unsupported tiffMode: %s", tiffMode)
	}

	return nil
}

func encodeRawCCITTG4(bits []byte, width, height int) ([]byte, error) {
	if len(bits) != ((width+7)/8)*height {
		return nil, errors.New("invalid packed bits length")
	}
	var outPtr *C.uchar
	var outSize C.ulong
	ret := C.encode_raw_g4(
		(*C.uchar)(unsafe.Pointer(&bits[0])),
		C.size_t(width), C.size_t(height),
		&outPtr, &outSize,
	)
	if ret != 0 {
		return nil, errors.New("libtiff CCITT-G4 encode failed")
	}
	defer C.free(unsafe.Pointer(outPtr))
	size := int(outSize)
	return C.GoBytes(unsafe.Pointer(outPtr), C.int(size)), nil
}

// // #cgo LDFLAGS: -ltiff -ljpeg -lwebp -lzstd -llzma -ldeflate -ljbig -lLerc -lz
// #cgo LDFLAGS: -static -ljpeg -ltiff
//#cgo LDFLAGS: -ljpeg -ltiff
