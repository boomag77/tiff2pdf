
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <emmintrin.h> // SSE2

#include "converter.h"
#include "settings.h"

// Convert RGB to grayscale using SSE2
void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, size_t npixels, int* ccitt_ready) {

    const __m128i coeff_r = _mm_set1_epi16(77);
    const __m128i coeff_g = _mm_set1_epi16(150);
    const __m128i coeff_b = _mm_set1_epi16(29);
    const __m128i zero = _mm_setzero_si128();

    size_t bw_pixels = npixels;

    for (size_t i = 0; i + 8 <= npixels; i += 8) {
        uint8_t r0[8], g0[8], b0[8];

        for (size_t j = 0; j < 8; j++) {
            r0[j] = rgb[(i + j) * 3 + 0];
            g0[j] = rgb[(i + j) * 3 + 1];
            b0[j] = rgb[(i + j) * 3 + 2];
        }

        __m128i r = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)r0), zero);
        __m128i g = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)g0), zero);
        __m128i b = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)b0), zero);

        r = _mm_mullo_epi16(r, coeff_r); // ++
        g = _mm_mullo_epi16(g, coeff_g); // ++
        b = _mm_mullo_epi16(b, coeff_b); // ++
        __m128i sum = _mm_add_epi16(_mm_add_epi16(r, g), b); // ++
        sum = _mm_srli_epi16(sum, 8); // ++

        __m128i res8 = _mm_packus_epi16(sum, zero);
        _mm_storel_epi64((__m128i*)(gray + i), res8);

        uint8_t tmp[8];
        _mm_storel_epi64((__m128i*)tmp, res8);
        for (size_t j = 0; j < 8; j++) {
            if (tmp[j] > LOWER_THRESHOLD && tmp[j] < UPPER_THRESHOLD) {
                bw_pixels--;
            }
        }
    }

    size_t tail = (npixels / 8) * 8;
    for (size_t i = tail; i < npixels; i++) {
        uint8_t r = rgb[i * 3 + 0];
        uint8_t g = rgb[i * 3 + 1];
        uint8_t b = rgb[i * 3 + 2];
        gray[i] = (r * 77 + g * 150 + b * 29) >> 8;
        if (gray[i] > LOWER_THRESHOLD && gray[i] < UPPER_THRESHOLD) {
            //bad_count++;
            bw_pixels--;
        }
    }

    *ccitt_ready = (bw_pixels * 100 >= npixels * CCITT_THRESHOLD);

}