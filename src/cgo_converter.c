#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>
#include <math.h>

#include <emmintrin.h> // SSE2

#ifndef TIFFTAG_JPEGCOLORMODE
#define TIFFTAG_JPEGCOLORMODE 65538
#endif

#ifndef JPEGCOLORMODE_RGB
#define JPEGCOLORMODE_RGB     1
#endif

#define GRAY_THRESHOLD 2

typedef struct {
    unsigned char* data;
    toff_t size;
    toff_t capacity;
    toff_t offset;
} MemoryWriter;

tsize_t memWrite(thandle_t handle, void* buf, tsize_t size) {
    MemoryWriter* mw = (MemoryWriter*)handle;
    if (mw->offset + size > mw->capacity) {
        toff_t new_capacity = mw->capacity * 2 + size;
        unsigned char* new_data = realloc(mw->data, new_capacity);
        if (!new_data) return 0;
        mw->data = new_data;
        mw->capacity = new_capacity;
    }
    memcpy(mw->data + mw->offset, buf, size);
    mw->offset += size;
    if (mw->offset > mw->size) {
        mw->size = mw->offset;
    }
    return size;
}

tsize_t memRead(thandle_t handle, void* buf, tsize_t size) {
    MemoryWriter* mw = (MemoryWriter*)handle;
    if (mw->offset + size > mw->size) {
        size = mw->size - mw->offset;
    }
    memcpy(buf, mw->data + mw->offset, size);
    mw->offset += size;
    return size;
}

toff_t memSeek(thandle_t handle, toff_t offset, int whence) {
    MemoryWriter* mw = (MemoryWriter*)handle;
    switch (whence) {
    case SEEK_SET:
        mw->offset = offset;
        break;
    case SEEK_CUR:
        mw->offset += offset;
        break;
    case SEEK_END:
        mw->offset = mw->size + offset;
        break;
    }
    return mw->offset;
}

int memClose(thandle_t handle) {
    return 0;
}

toff_t memSize(thandle_t handle) {
    MemoryWriter* mw = (MemoryWriter*)handle;
    return mw->size;
}




void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, int npixels, int* ccitt_ready) {
    const __m128i coeff_r = _mm_set1_epi16(30);
    const __m128i coeff_g = _mm_set1_epi16(59);
    const __m128i coeff_b = _mm_set1_epi16(11);
    const __m128i zero = _mm_setzero_si128();

    int i = 0;
    for (; i <= npixels - 8; i += 8) {
        uint8_t r0[8], g0[8], b0[8];

        for (int j = 0; j < 8; j++) {
            r0[j] = rgb[(i + j) * 3 + 0];
            g0[j] = rgb[(i + j) * 3 + 1];
            b0[j] = rgb[(i + j) * 3 + 2];
        }

        __m128i r = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)r0), zero);
        __m128i g = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)g0), zero);
        __m128i b = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)b0), zero);

        r = _mm_mullo_epi16(r, coeff_r);
        g = _mm_mullo_epi16(g, coeff_g);
        b = _mm_mullo_epi16(b, coeff_b);

        __m128i sum = _mm_add_epi16(r, g);
        sum = _mm_add_epi16(sum, b);

        sum = _mm_srli_epi16(sum, 6);

        __m128i res8 = _mm_packus_epi16(sum, zero);
        _mm_storel_epi64((__m128i*)(gray + i), res8);
    }

    int bad_count = 0;

    for (; i < npixels; i++) {
        uint8_t r = rgb[i * 3 + 0];
        uint8_t g = rgb[i * 3 + 1];
        uint8_t b = rgb[i * 3 + 2];
        gray[i] = (r * 30 + g * 59 + b * 11) / 100;
        if (gray[i] > 50 && gray[i] < 200) {
            bad_count++;
        }
    }
    double bad_ratio = (double)bad_count / (double)(npixels);
    if (bad_ratio < 0.05) {
        *ccitt_ready = 1;
    } else {
        *ccitt_ready = 0;
    }

}

// JPEG encoder from RGBA → JPEG memory
int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer,
                      int quality, int dpi, int gray,
                      unsigned char** out, unsigned long* outSize) {
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    JSAMPROW row_pointer[1];
    int row_stride = width * (gray ? 1 : 3);

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
int convert_tiff_to_data(const char* path,
                        int quality, int dpi, double scale,
                         unsigned char** outBuf, unsigned long* outSize,
                         int* use_ccitt, int* gray_filter,
                         int* outWidth, int* outHeight, int* outDpi)
{

    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

	TIFFSetWarningHandler(NULL);

    uint32_t orig_width = 0, orig_height = 0;
    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &orig_width) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &orig_height)) {
        TIFFClose(tif);
        return -2;
    }


    uint32_t* raster = _TIFFmalloc((size_t)orig_width * (size_t)orig_height * sizeof(uint32_t));
    if (!raster) {
        TIFFClose(tif);
        return -3;
    }

    if (!TIFFReadRGBAImageOriented(tif, orig_width, orig_height, raster, ORIENTATION_TOPLEFT, 0)) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -4;
    }

    int width = orig_width;
    int height = orig_height;

    if (scale != 1.0) {
        width = (uint32_t)(width * scale);
        height = (uint32_t)(height * scale);
    }

    int npixels = width * height;

    if (width == 0 || height == 0) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -5;
    }

    uint8_t* rgb = malloc(npixels * 3);
    if (!rgb) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -6;
    }
    uint8_t* gray = malloc(npixels);
    if (!gray) {
        fprintf(stderr, "malloc(gray) failed: %d bytes\n", npixels);
        free(rgb);
        _TIFFfree(raster);
        TIFFClose(tif);
        return -7;
    }

    int dst, dst_x, dst_y;
    uint32_t px;
    uint8_t r, g, b;

    int graycount = 0;
    int px_index;

    for (int y = 0; y < height; y++) {
        for (int x = 0; x < width; x++) {

            dst_x = (int)(floor(x / scale));
            dst_y = (int)(floor(y / scale));

            if (dst_x >= orig_width) {
                dst_x = orig_width - 1;
            }
            if (dst_y >= orig_height) {
                dst_y = orig_height - 1;
            }
            px = raster[dst_y * orig_width + dst_x];
            px_index = y * width + x;

            dst = px_index * 3;

            r = TIFFGetR(px);
            g = TIFFGetG(px);
            b = TIFFGetB(px);
            rgb[dst + 0] = r;
            rgb[dst + 1] = g;
            rgb[dst + 2] = b;
            if (abs(r-g) < GRAY_THRESHOLD && abs(r-b) < GRAY_THRESHOLD && abs(g-b) < GRAY_THRESHOLD) {
                graycount++;
            }
            //gray[px_index] = (uint8_t)((r * 30 + g * 59 + b * 11) / 100);

        }
    }

    double gray_ratio = (double)graycount / (double)(npixels);
    int use_gray = (gray_ratio > 0.9);

    int ccitt_ready = 0;
    if (use_gray) {
        rgb_to_gray_sse2(rgb, gray, npixels, &ccitt_ready);
    }

    int rc = -1;
    //ccitt_ready = 0; // exclude CCITT for now

    use_ccitt = ccitt_ready ? 1 : 0;

    rc = write_jpeg_to_mem(width, height, use_gray ? gray : rgb, quality, dpi, use_gray, outBuf, outSize);

    *outWidth = width;
    *outHeight = height;
    *outDpi = dpi;

    free(rgb);
    free(gray);
    _TIFFfree(raster);
    TIFFClose(tif);

    return rc == 0 ? 0 : -6;
}

int gray_to_ccitt_g4(const uint8_t* gray, int width, int height, unsigned char** outBuf, unsigned long* outSize) {
    MemoryWriter mw = {0};
    mw.data = malloc(4096);
    mw.capacity = 4096;
    mw.size = 0;
    mw.offset = 0;

    TIFF* tif = TIFFClientOpen(
        "mem",
        "w",
        (thandle_t)&mw,
        memRead,
        memWrite,
        memSeek,
        memClose,
        memSize,
        NULL,
        NULL
    );

    if (!tif) return -1;

    TIFFSetField(tif, TIFFTAG_IMAGEWIDTH, width);
    TIFFSetField(tif, TIFFTAG_IMAGELENGTH, height);
    TIFFSetField(tif, TIFFTAG_BITSPERSAMPLE, 1);
    TIFFSetField(tif, TIFFTAG_COMPRESSION, COMPRESSION_CCITTFAX4);
    TIFFSetField(tif, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISWHITE);
    TIFFSetField(tif, TIFFTAG_SAMPLESPERPIXEL, 1);
    TIFFSetField(tif, TIFFTAG_ROWSPERSTRIP, height);

    int rowbytes = (width + 7) / 8;
    for (int y = 0; y < height; y++) {
        TIFFWriteScanline(tif, (void*)(gray + y * rowbytes), y, 0);
    }

    TIFFClose(tif);

    *outBuf = mw.data;
    *outSize = mw.size;

    return 0;
}