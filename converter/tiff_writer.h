#ifndef TIFF_WRITER_H
#define TIFF_WRITER_H

#include <stdint.h>
#include <stddef.h>

void write_tiff(const char* filename,
                    uint32_t width, uint32_t height,
                    unsigned char* buf, size_t buf_size,
                    int dpi, int compression, int gray);

#endif