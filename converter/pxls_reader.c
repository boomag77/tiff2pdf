
#include <stdlib.h>
#include <math.h>
#include <tiffio.h>

#include "converter.h"
#include "settings.h"

// read pixels to rgb
int read_pxls_from_raster(uint32_t* raster, size_t* width, size_t* height,
                        uint8_t** pxls_buff, bool* gray, int* ccitt_ready)
{

    int ccitt_mode = *ccitt_ready;

    if (!raster || !pxls_buff || !gray || !ccitt_ready) {
        return -1;
    }
    if ((*width) == 0 || (*height) == 0) {
        return -2;
    }
    if ((*width) > SIZE_MAX / (*height)) {
        return -3;
    }
    size_t npixels = (*width) * (*height);

    if (npixels > SIZE_MAX / 3) {
        return -4;
    }

    uint32_t px;
    uint8_t r, g, b;
    size_t dst;
    size_t gr_count = 0;
    uint8_t* rgb = malloc(npixels * 3);
    if (!rgb) {
        return -5;
    }

    for (size_t y = 0; y < (*height); y++) {
        for (size_t x = 0; x < (*width); x++) {

            px = raster[y * (*width) + x];
            dst = (y * (*width) + x) * 3;

            r = TIFFGetR(px);
            g = TIFFGetG(px);
            b = TIFFGetB(px);
            rgb[dst + 0] = r;
            rgb[dst + 1] = g;
            rgb[dst + 2] = b;
            if (abs(r-g) < GRAY_THRESHOLD && abs(r-b) < GRAY_THRESHOLD && abs(g-b) < GRAY_THRESHOLD) {
                gr_count++;
            }
        }
    }

    double gray_ratio = (double)gr_count / (double)(npixels);
    if (gray_ratio > GRAY_RATIO || ccitt_mode == 1) {
        uint8_t* gray_buff =  malloc(npixels);
        if (!gray_buff) {
            free(rgb);
            return -6;
        }
        int ready_for_ccitt = 0;
        rgb_to_gray_sse2(rgb, gray_buff, npixels, &ready_for_ccitt);

        if (ccitt_mode == -1) {
            ready_for_ccitt = 0;
        }
        if (ccitt_mode == 1) {
            ready_for_ccitt = 1;
        }

        *pxls_buff = gray_buff;
        *gray = true;
        *ccitt_ready = ready_for_ccitt;

        free(rgb);
        return 0;
    }

    *pxls_buff = rgb;
    *gray = false;
    *ccitt_ready = false;

    return 0;
}

// read pixels to rgb_resampled
int read_pxls_resampled_from_raster(uint32_t* raster, size_t* width, size_t* height,
                                    uint8_t** pxls_buff, bool* gray, int* ccitt_ready,
                                    int target_dpi, int orig_dpi)
{
    int ccitt_mode = *ccitt_ready;

    if (!raster || !pxls_buff || !gray || !ccitt_ready) {
        return -1;
    }
    if ((*width) == 0 || (*height) == 0) {
        return -2;
    }
    if ((*width) > SIZE_MAX / (*height)) {
        return -3;
    }
    size_t npixels = (*width) * (*height);

    if (npixels > SIZE_MAX / 3) {
        return -4;
    }

    double resample_scale = (double)target_dpi / (double)orig_dpi;
    size_t new_width  = (*width)  * resample_scale + 0.5;
    size_t new_height = (*height) * resample_scale + 0.5;

    npixels = new_width * new_height;
    if (npixels > SIZE_MAX / 3) {
        return -5;
    }

    uint8_t* rgb_resampled = malloc(npixels * 3);
    if (!rgb_resampled) {
        return -6;
    }

    uint32_t px;
    uint8_t r, g, b;
    size_t gr_count = 0;
    size_t px_index;

    // bilinear resampling
    for (size_t y = 0; y < new_height; y++) {
        double fy = y / resample_scale;
        size_t    y0 = (size_t)floor(fy);
        double wy = fy - y0;
        if (y0 < 0)       { y0 = 0;    wy = 0; }
        if (y0 >= (*height)-1) { y0 = (*height)-2; wy = 1; }
        int y1 = y0 + 1;

        for (size_t x = 0; x < new_width; x++) {
            double fx = x / resample_scale;
            size_t    x0 = (size_t)floor(fx);
            double wx = fx - x0;
            if (x0 < 0)      { x0 = 0;    wx = 0; }
            if (x0 >= (*width)-1)  { x0 = (*width)-2;  wx = 1; }
            int x1 = x0 + 1;

            // 4 pixel neighbors
            uint32_t p00 = raster[y0*(*width) + x0];
            uint32_t p10 = raster[y0*(*width) + x1];
            uint32_t p01 = raster[y1*(*width) + x0];
            uint32_t p11 = raster[y1*(*width) + x1];

            // get channels
            uint8_t r00 = TIFFGetR(p00), g00 = TIFFGetG(p00), b00 = TIFFGetB(p00);
            uint8_t r10 = TIFFGetR(p10), g10 = TIFFGetG(p10), b10 = TIFFGetB(p10);
            uint8_t r01 = TIFFGetR(p01), g01 = TIFFGetG(p01), b01 = TIFFGetB(p01);
            uint8_t r11 = TIFFGetR(p11), g11 = TIFFGetG(p11), b11 = TIFFGetB(p11);

            // linear interpolation X
            double r0 = r00*(1-wx) + r10*wx;
            double g0 = g00*(1-wx) + g10*wx;
            double b0 = b00*(1-wx) + b10*wx;
            double r1 = r01*(1-wx) + r11*wx;
            double g1 = g01*(1-wx) + g11*wx;
            double b1 = b01*(1-wx) + b11*wx;

            // linear interpolation Y
            uint8_t rf = (uint8_t)(r0*(1-wy) + r1*wy + 0.5);
            uint8_t gf = (uint8_t)(g0*(1-wy) + g1*wy + 0.5);
            uint8_t bf = (uint8_t)(b0*(1-wy) + b1*wy + 0.5);

            px_index = y * new_width + x;
            rgb_resampled[px_index*3 + 0] = rf;
            rgb_resampled[px_index*3 + 1] = gf;
            rgb_resampled[px_index*3 + 2] = bf;
            if (abs(rf-gf) < GRAY_THRESHOLD && abs(rf-bf) < GRAY_THRESHOLD && abs(gf-bf) < GRAY_THRESHOLD) {
                gr_count++;
            }

        }
    }

    *width = new_width;
    *height = new_height;

    double gray_ratio = (double)gr_count / (double)(npixels);
    if (gray_ratio > GRAY_RATIO || ccitt_mode == 1) {
        uint8_t* gray_buff =  malloc(npixels);

        if (!gray_buff) {
            free(rgb_resampled);
            return -7;
        }
        int ready_for_ccitt = 0;
        rgb_to_gray_sse2(rgb_resampled, gray_buff, npixels, &ready_for_ccitt);

        if (ccitt_mode == -1) {
            ready_for_ccitt = 0;
        }
        if (ccitt_mode == 1) {
            ready_for_ccitt = 1;
        }

        *pxls_buff = gray_buff;
        *gray = true;
        *ccitt_ready = ready_for_ccitt;

        free(rgb_resampled);
        return 0;
    }

    *pxls_buff = rgb_resampled;
    *gray = false;
    *ccitt_ready = false;

    return 0;
}