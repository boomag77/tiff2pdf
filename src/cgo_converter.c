#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>
#include <math.h>

#ifndef TIFFTAG_JPEGCOLORMODE
#define TIFFTAG_JPEGCOLORMODE 65538
#endif

#ifndef JPEGCOLORMODE_RGB
#define JPEGCOLORMODE_RGB     1
#endif

#define GRAY_THRESHOLD 12

// JPEG encoder from RGBA → JPEG memory
int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer,
                      int quality, int dpi, int gray,
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
    cinfo.input_components = gray ? 1 : 3;
    cinfo.in_color_space = gray ? JCS_GRAYSCALE : JCS_RGB;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, quality, TRUE);
    jpeg_simple_progression(&cinfo);
    cinfo.density_unit = 1;
    cinfo.X_density = dpi;
    cinfo.Y_density = dpi;

    if (!gray) {
        cinfo.comp_info[0].h_samp_factor = 2;
        cinfo.comp_info[0].v_samp_factor = 2;
        cinfo.comp_info[1].h_samp_factor = 1;
        cinfo.comp_info[1].v_samp_factor = 1;
        cinfo.comp_info[2].h_samp_factor = 1;
        cinfo.comp_info[2].v_samp_factor = 1;
    }

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
int convert_tiff_to_jpeg(const char* path, int quality, int dpi, double scale,
                         unsigned char** outBuf, unsigned long* outSize,
                         int* outWidth, int* outHeight, int* outDpi)
{

    printf("Start convert_tiff_to_jpeg\n");

    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

	TIFFSetWarningHandler(NULL);

    uint32_t width = 0, height = 0;
    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &width) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &height)) {
        TIFFClose(tif);
        return -2;
    }



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

    

    int scaled_width = (int)(width * scale);
    int scaled_height = (int)(height * scale);
    int row_stride = scaled_width * 3;

    int npixels_scaled = scaled_width * scaled_height;

    if (npixels_scaled <= 0) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -5;
    }

    uint8_t* rgb = malloc(npixels_scaled* 3);
    if (!rgb) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -6;
    }
    uint8_t* gray = malloc(npixels_scaled);
    if (!gray) {
        free(rgb);
        _TIFFfree(raster);
        TIFFClose(tif);
        return -7;
    }

    int graycount = 0;

    for (int y = 0; y < scaled_height; y++) {
        for (int x = 0; x < scaled_width; x++) {
            int orig_x = (int)floor((double)x / scale);
            int orig_y = (int)floor((double)y / scale);
            if (orig_x >= width) orig_x = width - 1;
            if (orig_y >= height) orig_y = height - 1;
            uint32_t px = raster[orig_y * width + orig_x];
            int dst = (y * scaled_width + x) * 3;
            uint8_t r = TIFFGetR(px);
            uint8_t g = TIFFGetG(px);
            uint8_t b = TIFFGetB(px);
            rgb[dst + 0] = r;
            rgb[dst + 1] = g;
            rgb[dst + 2] = b;
            if (abs(r-g) < GRAY_THRESHOLD && abs(r-b) < GRAY_THRESHOLD && abs(g-b) < GRAY_THRESHOLD) {
                graycount++;
            }
            gray[y * scaled_width + x] = (uint8_t)((r * 30 + g * 59 + b * 11) / 100);
        }
    }

    double gray_ratio = (double)graycount / (double)(scaled_width * scaled_height);
    int use_gray = (gray_ratio > 0.9);

    int rc = write_jpeg_to_mem(scaled_width, scaled_height, use_gray ? gray : rgb, quality, dpi, use_gray, outBuf, outSize);

    *outWidth = scaled_width;
    *outHeight = scaled_height;
    *outDpi = dpi;

    free(rgb);
    free(gray);
    _TIFFfree(raster);
    TIFFClose(tif);

    return rc == 0 ? 0 : -6;
}