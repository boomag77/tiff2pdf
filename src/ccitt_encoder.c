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

static const HuffmanEntry white_terminating[] = {
    {0x35, 8, 0}, {0x7, 6, 1}, {0x7, 4, 2}, {0x8, 4, 3},
    {0xB, 4, 4}, {0xC, 4, 5}, {0xE, 4, 6}, {0xF, 4, 7},
    {0x13, 5, 8}, {0x14, 5, 9}, {0x7, 5, 10}, {0x8, 5, 11},
    {0x8, 6, 12}, {0x3, 6, 13}, {0x34, 6, 14}, {0x35, 6, 15},
    {0x2A, 6, 16}, {0x2B, 6, 17}, {0x27, 7, 18}, {0xC, 7, 19},
    {0x8, 7, 20}, {0x17, 7, 21}, {0x3, 7, 22}, {0x4, 7, 23},
    {0x28, 7, 24}, {0x2B, 7, 25}, {0x13, 7, 26}, {0x24, 7, 27},
    {0x18, 7, 28}, {0x2, 8, 29}, {0x3, 8, 30}, {0x1A, 8, 31},
    {0x1B, 8, 32}, {0x12, 8, 33}, {0x13, 8, 34}, {0x14, 8, 35},
    {0x15, 8, 36}, {0x16, 8, 37}, {0x17, 8, 38}, {0x28, 8, 39},
    {0x29, 8, 40}, {0x2A, 8, 41}, {0x2B, 8, 42}, {0x2C, 8, 43},
    {0x2D, 8, 44}, {0x4, 8, 45}, {0x5, 8, 46}, {0xA, 8, 47},
    {0xB, 8, 48}, {0x52, 8, 49}, {0x53, 8, 50}, {0x54, 8, 51},
    {0x55, 8, 52}, {0x24, 8, 53}, {0x25, 8, 54}, {0x58, 8, 55},
    {0x59, 8, 56}, {0x5A, 8, 57}, {0x5B, 8, 58}, {0x4A, 8, 59},
    {0x4B, 8, 60}, {0x32, 8, 61}, {0x33, 8, 62}, {0x34, 8, 63},
};

static const HuffmanEntry black_terminating[] = {
    {0x37, 10, 0}, {0x2, 3, 1}, {0x3, 2, 2}, {0x2, 2, 3},
    {0x3, 3, 4}, {0x3, 4, 5}, {0x2, 4, 6}, {0x3, 5, 7},
    {0x5, 6, 8}, {0x4, 6, 9}, {0x4, 7, 10}, {0x5, 7, 11},
    {0x7, 7, 12}, {0x4, 8, 13}, {0x7, 8, 14}, {0x18, 9, 15},
    {0x17, 10, 16}, {0x18, 10, 17}, {0x8, 10, 18}, {0x67, 11, 19},
    {0x68, 11, 20}, {0x6C, 11, 21}, {0x37, 11, 22}, {0x28, 11, 23},
    {0x17, 11, 24}, {0x18, 11, 25}, {0xCA, 12, 26}, {0xCB, 12, 27},
    {0xCC, 12, 28}, {0xCD, 12, 29}, {0x68, 12, 30}, {0x69, 12, 31},
    {0x6A, 12, 32}, {0x6B, 12, 33}, {0xD2, 12, 34}, {0xD3, 12, 35},
    {0xD4, 12, 36}, {0xD5, 12, 37}, {0xD6, 12, 38}, {0xD7, 12, 39},
    {0x6C, 12, 40}, {0x6D, 12, 41}, {0xDA, 12, 42}, {0xDB, 12, 43},
    {0x54, 12, 44}, {0x55, 12, 45}, {0x56, 12, 46}, {0x57, 12, 47},
    {0x64, 12, 48}, {0x65, 12, 49}, {0x52, 12, 50}, {0x53, 12, 51},
    {0x24, 12, 52}, {0x37, 12, 53}, {0x38, 12, 54}, {0x27, 12, 55},
    {0x28, 12, 56}, {0x58, 12, 57}, {0x59, 12, 58}, {0x2B, 12, 59},
    {0x2C, 12, 60}, {0x5A, 12, 61}, {0x66, 12, 62}, {0x67, 12, 63},
};

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

static void analyze_line(const uint8_t* bitmap, int width, int y, int* run_lengths, int* run_count) {
    int color = 0; // 0 - белый старт
    int run = 0;
    *run_count = 0;
    for (int x = 0; x < width; x++) {
        int bit = get_bit(bitmap, width, x, y);
        if (bit == color) {
            run++;
        } else {
            run_lengths[(*run_count)++] = run;
            run = 1;
            color = bit;
        }
    }
    run_lengths[(*run_count)++] = run;
}

static void encode_line(BitWriter* bw, const uint8_t* ref_bitmap, const uint8_t* cur_bitmap, int width, int y) {
    int a0 = 0;
    int a1, a2;
    int b1, b2;
    int color = 0; // начинаем с белого

    while (a0 < width) {
        // Найти b1 и b2 на reference line
        b1 = a0;
        while (b1 < width && get_bit(ref_bitmap, width, b1, y - 1) == color) {
            b1++;
        }
        b2 = b1;
        while (b2 < width && get_bit(ref_bitmap, width, b2, y - 1) != color) {
            b2++;
        }

        // Найти a1 и a2 на текущей строке
        a1 = a0;
        while (a1 < width && get_bit(cur_bitmap, width, a1, y) == color) {
            a1++;
        }
        a2 = a1;
        while (a2 < width && get_bit(cur_bitmap, width, a2, y) != color) {
            a2++;
        }

        if (b2 <= a1) {
            // PASS MODE
            bitwriter_write_bits(bw, 0x1, 4); // код PASS: 0001
            a0 = b2;
        } else {
            int d = b1 - a1;
            if (d >= -3 && d <= 3) {
                // VERTICAL MODE
                static const uint16_t vert_codes[7] = { 0x03, 0x03, 0x07, 0x0F, 0x17, 0x1F, 0x37 };
                static const int vert_lengths[7] = { 7, 6, 3, 1, 3, 6, 7 };
                int idx = d + 3;
                bitwriter_write_bits(bw, vert_codes[idx], vert_lengths[idx]);
                a0 = a1;
                color = 1 - color;
            } else {
                // HORIZONTAL MODE
                bitwriter_write_bits(bw, 0x1, 3); // код HORIZONTAL: 001

                int white_run = 0;
                for (int i = a0; i < width; i++) {
                    int bit = get_bit(cur_bitmap, width, i, y);
                    if (bit != color) break;
                    white_run++;
                }

                int black_run = 0;
                for (int i = a0 + white_run; i < width; i++) {
                    int bit = get_bit(cur_bitmap, width, i, y);
                    if (bit == color) break;
                    black_run++;
                }

                // Кодируем длины через encode_run_length
                encode_run_length(bw, white_run, color);
                encode_run_length(bw, black_run, 1 - color);

                a0 += white_run + black_run;
            }
        }
    }
}


static void encode_run_length(BitWriter* bw, int run_length, int color) {
    const HuffmanEntry* table = color == 0 ? white_terminating : black_terminating;

    // Сначала обрабатываем make-up codes (если run_length >= 64)
    while (run_length >= 64) {
        int makeup_length = (run_length / 64) * 64; // округляем вниз до кратного 64
        int makeup_run = makeup_length;

        uint16_t code = 0;
        uint8_t length = 0;

        // Поиск make-up кода
        if (makeup_run == 64) {
            code = 0x1b; length = 5;
        } else if (makeup_run == 128) {
            code = 0x12; length = 5;
        } else if (makeup_run == 192) {
            code = 0x17; length = 6;
        } else if (makeup_run == 256) {
            code = 0x37; length = 7;
        } else if (makeup_run == 320) {
            code = 0x36; length = 8;
        } else if (makeup_run == 384) {
            code = 0x37; length = 8;
        } else if (makeup_run == 448) {
            code = 0x64; length = 8;
        } else if (makeup_run == 512) {
            code = 0x65; length = 8;
        } else if (makeup_run == 576) {
            code = 0x68; length = 8;
        } else if (makeup_run == 640) {
            code = 0x67; length = 8;
        } else if (makeup_run == 704) {
            code = 0xCC; length = 9;
        } else if (makeup_run == 768) {
            code = 0xCD; length = 9;
        } else if (makeup_run == 832) {
            code = 0xD2; length = 9;
        } else if (makeup_run == 896) {
            code = 0xD3; length = 9;
        } else if (makeup_run == 960) {
            code = 0xD4; length = 9;
        } else if (makeup_run == 1024) {
            code = 0xD5; length = 9;
        } else if (makeup_run == 1088) {
            code = 0xD6; length = 9;
        } else if (makeup_run == 1152) {
            code = 0xD7; length = 9;
        } else if (makeup_run == 1216) {
            code = 0xD8; length = 9;
        } else if (makeup_run == 1280) {
            code = 0xD9; length = 9;
        } else if (makeup_run == 1344) {
            code = 0xDA; length = 9;
        } else if (makeup_run == 1408) {
            code = 0xDB; length = 9;
        } else if (makeup_run == 1472) {
            code = 0x98; length = 9;
        } else if (makeup_run == 1536) {
            code = 0x99; length = 9;
        } else if (makeup_run == 1600) {
            code = 0x9A; length = 9;
        } else if (makeup_run == 1664) {
            code = 0x18; length = 6;
        } else if (makeup_run == 1728) {
            code = 0x17; length = 5;
        }

        // Пишем make-up код
        if (code != 0 && length != 0) {
            bitwriter_write_bits(bw, code, length);
            run_length -= makeup_run;
        } else {
            // Ошибка! Некорректная длина
            break;
        }
    }

    // Теперь остаток (terminating code) для run_length < 64
    for (int i = 0; i < 64; i++) {
        if (table[i].run_length == run_length) {
            bitwriter_write_bits(bw, table[i].code, table[i].length);
            return;
        }
    }
}

