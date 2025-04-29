#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    uint8_t* buffer;
    unsigned long size;
    unsigned long capacity;
    int bit_pos;
    uint8_t current_byte;
} BitWriter;

typedef struct {
    uint16_t code;
    uint8_t length;
    int run_length;
} HuffmanEntry;

// Таблицы терминальных кодов (уже были даны выше)
extern const HuffmanEntry white_terminating[];
extern const HuffmanEntry black_terminating[];

static void bitwriter_init(BitWriter* bw) {
    bw->capacity = 4096;
    bw->size = 0;
    bw->buffer = (uint8_t*)malloc(bw->capacity);
    bw->bit_pos = 0;
    bw->current_byte = 0;
}

static void bitwriter_write_bit(BitWriter* bw, int bit) {
    if (bw->bit_pos == 8) {
        if (bw->size >= bw->capacity) {
            bw->capacity *= 2;
            bw->buffer = (uint8_t*)realloc(bw->buffer, bw->capacity);
        }
        bw->buffer[bw->size++] = bw->current_byte;
        bw->current_byte = 0;
        bw->bit_pos = 0;
    }
    bw->current_byte <<= 1;
    if (bit) {
        bw->current_byte |= 1;
    }
    bw->bit_pos++;
}

static void bitwriter_write_bits(BitWriter* bw, uint16_t code, int length) {
    for (int i = length - 1; i >= 0; i--) {
        bitwriter_write_bit(bw, (code >> i) & 1);
    }
}

static void bitwriter_flush(BitWriter* bw) {
    if (bw->bit_pos > 0) {
        bw->current_byte <<= (8 - bw->bit_pos);
        if (bw->size >= bw->capacity) {
            bw->capacity *= 2;
            bw->buffer = (uint8_t*)realloc(bw->buffer, bw->capacity);
        }
        bw->buffer[bw->size++] = bw->current_byte;
    }
}

static int get_bit(const uint8_t* bitmap, int width, int x, int y) {
    int byte_idx = y * ((width + 7) / 8) + (x / 8);
    int bit_idx = 7 - (x % 8);
    return (bitmap[byte_idx] >> bit_idx) & 1;
}

static void encode_run_length(BitWriter* bw, int run_length, int color) {
    const HuffmanEntry* table = color == 0 ? white_terminating : black_terminating;

    // Makeup коды
    while (run_length >= 64) {
        int makeup_run = (run_length / 64) * 64;

        uint16_t code = 0;
        uint8_t length = 0;

        // Стандартные makeup-коды для CCITT Group 4
        if (makeup_run == 64) { code = 0x1b; length = 5; }
        else if (makeup_run == 128) { code = 0x12; length = 5; }
        else if (makeup_run == 192) { code = 0x17; length = 6; }
        else if (makeup_run == 256) { code = 0x37; length = 7; }
        else if (makeup_run == 320) { code = 0x36; length = 8; }
        else if (makeup_run == 384) { code = 0x37; length = 8; }
        else if (makeup_run == 448) { code = 0x64; length = 8; }
        else if (makeup_run == 512) { code = 0x65; length = 8; }
        else if (makeup_run == 576) { code = 0x68; length = 8; }
        else if (makeup_run == 640) { code = 0x67; length = 8; }
        else if (makeup_run == 704) { code = 0xCC; length = 9; }
        else if (makeup_run == 768) { code = 0xCD; length = 9; }
        else if (makeup_run == 832) { code = 0xD2; length = 9; }
        else if (makeup_run == 896) { code = 0xD3; length = 9; }
        else if (makeup_run == 960) { code = 0xD4; length = 9; }
        else if (makeup_run == 1024) { code = 0xD5; length = 9; }
        else if (makeup_run == 1088) { code = 0xD6; length = 9; }
        else if (makeup_run == 1152) { code = 0xD7; length = 9; }
        else if (makeup_run == 1216) { code = 0xD8; length = 9; }
        else if (makeup_run == 1280) { code = 0xD9; length = 9; }
        else if (makeup_run == 1344) { code = 0xDA; length = 9; }
        else if (makeup_run == 1408) { code = 0xDB; length = 9; }
        else if (makeup_run == 1472) { code = 0x98; length = 9; }
        else if (makeup_run == 1536) { code = 0x99; length = 9; }
        else if (makeup_run == 1600) { code = 0x9A; length = 9; }
        else if (makeup_run == 1664) { code = 0x18; length = 6; }
        else if (makeup_run == 1728) { code = 0x17; length = 5; }

        if (code && length) {
            bitwriter_write_bits(bw, code, length);
            run_length -= makeup_run;
        } else {
            break;
        }
    }

    // Terminator
    for (int i = 0; i < 64; i++) {
        if (table[i].run_length == run_length) {
            bitwriter_write_bits(bw, table[i].code, table[i].length);
            break;
        }
    }
}

static void encode_line(BitWriter* bw, const uint8_t* ref_bitmap, const uint8_t* cur_bitmap, int width, int y) {
    int a0 = 0;
    int a1, a2;
    int b1, b2;
    int color = 0; // белый старт

    while (a0 < width) {
        b1 = a0;
        while (b1 < width && get_bit(ref_bitmap, width, b1, y-1) == color) b1++;
        b2 = b1;
        while (b2 < width && get_bit(ref_bitmap, width, b2, y-1) != color) b2++;

        a1 = a0;
        while (a1 < width && get_bit(cur_bitmap, width, a1, y) == color) a1++;
        a2 = a1;
        while (a2 < width && get_bit(cur_bitmap, width, a2, y) != color) a2++;

        if (b2 <= a1) {
            bitwriter_write_bits(bw, 0x1, 4); // PASS
            a0 = b2;
        } else {
            int d = b1 - a1;
            if (d >= -3 && d <= 3) {
                static const uint16_t vert_codes[7] = { 0x03, 0x03, 0x07, 0x0F, 0x17, 0x1F, 0x37 };
                static const int vert_lengths[7] = { 7, 6, 3, 1, 3, 6, 7 };
                int idx = d + 3;
                bitwriter_write_bits(bw, vert_codes[idx], vert_lengths[idx]);
                a0 = a1;
                color = 1 - color;
            } else {
                bitwriter_write_bits(bw, 0x1, 3); // HORIZONTAL
                int white_run = 0;
                for (int i = a0; i < width; i++) {
                    if (get_bit(cur_bitmap, width, i, y) != color) break;
                    white_run++;
                }
                int black_run = 0;
                for (int i = a0 + white_run; i < width; i++) {
                    if (get_bit(cur_bitmap, width, i, y) == color) break;
                    black_run++;
                }

                encode_run_length(bw, white_run, color);
                encode_run_length(bw, black_run, 1 - color);

                a0 += white_run + black_run;
            }
        }
    }
}

int encode_group4(const uint8_t* bitmap, int width, int height, uint8_t** outBuf, unsigned long* outSize) {
    if (width <= 0 || height <= 0) return -1;

    BitWriter bw;
    bitwriter_init(&bw);

    for (int y = 0; y < height; y++) {
        encode_line(&bw, bitmap, bitmap, width, y);
    }

    // End Of Block
    bitwriter_write_bits(&bw, 0x001, 12);

    bitwriter_flush(&bw);

    *outBuf = bw.buffer;
    *outSize = bw.size;

    return 0;
}
