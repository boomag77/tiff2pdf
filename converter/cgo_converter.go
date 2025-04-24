//go:build cgo
// +build cgo

package converter

/*

#cgo LDFLAGS: -ljpeg -ltiff

#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>

#ifndef TIFFTAG_JPEGCOLORMODE
#define TIFFTAG_JPEGCOLORMODE 65538
#endif

#ifndef JPEGCOLORMODE_RGB
#define JPEGCOLORMODE_RGB     1
#endif

// JPEG encoder from RGBA → JPEG memory
int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer,
                      int quality, int dpi,
                      unsigned char** out, unsigned long* outSize) {
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    JSAMPROW row_pointer[1];
    int row_stride = width * 3;

    cinfo.err = jpeg_std_error(&jerr);
    jpeg_create_compress(&cinfo);
    jpeg_mem_dest(&cinfo, out, outSize);

    cinfo.image_width = width;
    cinfo.image_height = height;
    cinfo.input_components = 3;
    cinfo.in_color_space = JCS_RGB;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, quality, TRUE);
    cinfo.density_unit = 1;
    cinfo.X_density = dpi;
    cinfo.Y_density = dpi;

    jpeg_start_compress(&cinfo, TRUE);
    while (cinfo.next_scanline < cinfo.image_height) {
        row_pointer[0] = &buffer[cinfo.next_scanline * row_stride];
        jpeg_write_scanlines(&cinfo, row_pointer, 1);
    }
    jpeg_finish_compress(&cinfo);
    jpeg_destroy_compress(&cinfo);

    return 0;
}

// TIFF → JPEG via RGBA
int convert_tiff_to_jpeg(const char* path, int quality,
                         unsigned char** outBuf, unsigned long* outSize, int* outWidth, int* outHeight, int* outDpi)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

	TIFFSetWarningHandler(NULL);

    uint32_t width = 0, height = 0;
    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &width) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &height)) {
        TIFFClose(tif);
        return -2;
    }


    float xres = 72.0;
    uint16_t res_unit = RESUNIT_INCH;
    TIFFGetField(tif, TIFFTAG_XRESOLUTION, &xres);
    TIFFGetField(tif, TIFFTAG_RESOLUTIONUNIT, &res_unit);
    int dpi = (res_unit == RESUNIT_CENTIMETER) ? (int)(xres * 2.54f) : (int)xres;

    size_t npixels = (size_t)width * (size_t)height;
    uint32_t* raster = _TIFFmalloc(npixels * sizeof(uint32_t));
    if (!raster) {
        TIFFClose(tif);
        return -3;
    }

    if (!TIFFReadRGBAImageOriented(tif, width, height, raster, ORIENTATION_TOPLEFT, 0)) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -4;
    }

    uint8_t* rgb = malloc(npixels * 3);
    if (!rgb) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -5;
    }

    for (size_t i = 0; i < npixels; i++) {
        uint32_t px = raster[i];
        rgb[3*i+0] = TIFFGetR(px);
        rgb[3*i+1] = TIFFGetG(px);
        rgb[3*i+2] = TIFFGetB(px);
    }

    int rc = write_jpeg_to_mem(width, height, rgb, quality, dpi, outBuf, outSize);

    *outWidth = width;
    *outHeight = height;
    *outDpi = dpi;

    free(rgb);
    _TIFFfree(raster);
    TIFFClose(tif);

    return rc == 0 ? 0 : -6;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// ConvertTIFFtoJPEG reads TIFF from file path and returns JPEG-encoded []byte + width, height, dpi
func ConvertTIFFtoJPEG(path string, quality int) (jpegData []byte, width, height, dpi int, err error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var outBuf *C.uchar
	var outSize C.ulong
	var w, h, d C.int

	rc := C.convert_tiff_to_jpeg(cPath, C.int(quality),
		&outBuf, &outSize,
		&w, &h, &d,
	)

	if rc != 0 {
		return nil, 0, 0, 0, fmt.Errorf("convert_tiff_to_jpeg failed with code %d", int(rc))
	}

	defer C.free(unsafe.Pointer(outBuf))
	jpegData = C.GoBytes(unsafe.Pointer(outBuf), C.int(outSize))
	return jpegData, int(w), int(h), int(d), nil
}

// // #cgo LDFLAGS: -ltiff -ljpeg -lwebp -lzstd -llzma -ldeflate -ljbig -lLerc -lz
// #cgo LDFLAGS: -static -ljpeg -ltiff
