#ifndef SFREGA_AUDIO_H
#define SFREGA_AUDIO_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct ma_dec ma_dec_t;
typedef struct ma_rb  ma_rb_t;
typedef struct ma_dev ma_dev_t;

/* Decoder */
ma_dec_t*     dec_open(const char* path);
uint64_t      dec_get_length_pcm(ma_dec_t* d);   /* total frames */
uint32_t      dec_get_channels(ma_dec_t* d);
uint32_t      dec_get_sample_rate(ma_dec_t* d);
uint64_t      dec_read_f32(ma_dec_t* d, float* buf, uint64_t frames);
bool          dec_seek_pcm(ma_dec_t* d, int64_t frame);
void          dec_close(ma_dec_t* d);

/* Ring buffer (float interleaved samples) */
ma_rb_t*      rb_create(uint64_t capacity_samples);
uint64_t      rb_write(ma_rb_t* rb, const float* data, uint64_t samples);
uint64_t      rb_read(ma_rb_t* rb, float* data, uint64_t samples);
uint64_t      rb_available(ma_rb_t* rb);
void          rb_clear(ma_rb_t* rb);
void          rb_destroy(ma_rb_t* rb);

/* Device */
ma_dev_t*     dev_open(uint32_t sample_rate, uint32_t channels, ma_rb_t* rb);
bool          dev_start(ma_dev_t* d);
void          dev_stop(ma_dev_t* d);
void          dev_set_volume(ma_dev_t* d, float volume); /* 0.0 - 1.0 */
float         dev_get_volume(ma_dev_t* d);
void          dev_close(ma_dev_t* d);

#ifdef __cplusplus
}
#endif

#endif
