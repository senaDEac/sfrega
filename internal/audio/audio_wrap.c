#define MINIAUDIO_IMPLEMENTATION
#include "miniaudio.h"
#include "audio.h"
#include <stdlib.h>
#include <string.h>

/* ---------- Decoder ---------- */

struct ma_dec {
    ma_decoder decoder;
    ma_uint32 channels;
    ma_uint32 sampleRate;
};

ma_dec_t* dec_open(const char* path) {
    ma_dec_t* d = (ma_dec_t*)calloc(1, sizeof(ma_dec_t));
    if (!d) return NULL;
    ma_decoder_config config = ma_decoder_config_init(ma_format_f32, 0, 0);
    ma_result r = ma_decoder_init_file(path, &config, &d->decoder);
    if (r != MA_SUCCESS) { free(d); return NULL; }
    ma_format fmt;
    ma_uint32 ch = 0, sr = 0;
    ma_decoder_get_data_format(&d->decoder, &fmt, &ch, &sr, NULL, 0);
    d->channels   = ch;
    d->sampleRate = sr;
    return d;
}

uint64_t dec_get_length_pcm(ma_dec_t* d) {
    ma_uint64 len = 0;
    ma_decoder_get_length_in_pcm_frames(&d->decoder, &len);
    return len;
}
uint32_t dec_get_channels(ma_dec_t* d)   { return d->channels; }
uint32_t dec_get_sample_rate(ma_dec_t* d){ return d->sampleRate; }

uint64_t dec_read_f32(ma_dec_t* d, float* buf, uint64_t frames) {
    if (!d || frames == 0) return 0;
    ma_uint64 framesRead = 0;
    ma_result r = ma_decoder_read_pcm_frames(&d->decoder, buf,
        (ma_uint64)frames, &framesRead);
    if (r != MA_SUCCESS && r != MA_AT_END) return 0;
    return framesRead;
}

bool dec_seek_pcm(ma_dec_t* d, int64_t frame) {
    if (frame < 0) frame = 0;
    return ma_decoder_seek_to_pcm_frame(&d->decoder, (ma_uint64)frame) == MA_SUCCESS;
}

void dec_close(ma_dec_t* d) {
    if (!d) return;
    ma_decoder_uninit(&d->decoder);
    free(d);
}

/* ---------- Ring buffer ---------- */

struct ma_rb {
    ma_mutex mutex;
    float* data;
    ma_uint64 capacity; /* samples */
    ma_uint64 rpos;     /* read position */
    ma_uint64 wpos;     /* write position */
    ma_uint64 count;    /* samples available */
};

ma_rb_t* rb_create(uint64_t capacity_samples) {
    ma_rb_t* rb = (ma_rb_t*)calloc(1, sizeof(ma_rb_t));
    if (!rb) return NULL;
    ma_mutex_init(&rb->mutex);
    rb->capacity = capacity_samples;
    rb->data = (float*)malloc(capacity_samples * sizeof(float));
    if (!rb->data) { free(rb); return NULL; }
    return rb;
}

uint64_t rb_write(ma_rb_t* rb, const float* data, uint64_t samples) {
    if (!rb || samples == 0) return 0;
    ma_mutex_lock(&rb->mutex);
    uint64_t space = rb->capacity - rb->count;
    if (samples > space) samples = space;
    uint64_t w = rb->wpos;
    uint64_t first = rb->capacity - w;
    if (first > samples) first = samples;
    memcpy(rb->data + w, data, first * sizeof(float));
    if (samples > first)
        memcpy(rb->data, data + first, (samples - first) * sizeof(float));
    rb->wpos = (w + samples) % rb->capacity;
    rb->count += samples;
    ma_mutex_unlock(&rb->mutex);
    return samples;
}

uint64_t rb_read(ma_rb_t* rb, float* data, uint64_t samples) {
    if (!rb) return 0;
    ma_mutex_lock(&rb->mutex);
    if (samples > rb->count) samples = rb->count;
    uint64_t rpos = rb->rpos;
    uint64_t first = rb->capacity - rpos;
    if (first > samples) first = samples;
    memcpy(data, rb->data + rpos, first * sizeof(float));
    if (samples > first)
        memcpy(data + first, rb->data, (samples - first) * sizeof(float));
    rb->rpos = (rpos + samples) % rb->capacity;
    rb->count -= samples;
    ma_mutex_unlock(&rb->mutex);
    return samples;
}

uint64_t rb_available(ma_rb_t* rb) {
    if (!rb) return 0;
    ma_mutex_lock(&rb->mutex);
    uint64_t n = rb->count;
    ma_mutex_unlock(&rb->mutex);
    return n;
}

void rb_clear(ma_rb_t* rb) {
    if (!rb) return;
    ma_mutex_lock(&rb->mutex);
    rb->rpos = rb->wpos = rb->count = 0;
    ma_mutex_unlock(&rb->mutex);
}

void rb_destroy(ma_rb_t* rb) {
    if (!rb) return;
    ma_mutex_uninit(&rb->mutex);
    free(rb->data);
    free(rb);
}

/* ---------- Device ---------- */

struct ma_dev {
    ma_device device;
    ma_rb_t* rb;
    volatile float volume;
    volatile float pan; /* -1 esquerda .. +1 direita */
    uint32_t channels;
};

static void dev_data_cb(ma_device* pDevice, void* pOutput, const void* pInput,
                        ma_uint32 frameCount) {
    (void)pInput;
    ma_dev_t* d = (ma_dev_t*)pDevice->pUserData;
    ma_uint32 channels = d->channels;
    ma_uint64 samples = (ma_uint64)frameCount * channels;
    float* out = (float*)pOutput;
    memset(out, 0, samples * sizeof(float));
    if (d->rb) {
        uint64_t got = rb_read(d->rb, out, samples);
        (void)got; /* remainder stays silent */
    }
    float pan = d->pan;
    float gl = 1.0f, gr = 1.0f;
    if (pan < 0.0f) {
        gr = 1.0f + pan;
    } else {
        gl = 1.0f - pan;
    }
    float g = d->volume;
    if (channels == 2) {
        for (ma_uint64 i = 0; i < samples; i += 2) {
            out[i]     *= g * gl;
            out[i + 1] *= g * gr;
        }
    } else {
        for (ma_uint64 i = 0; i < samples; i++) out[i] *= g;
    }
}

ma_dev_t* dev_open(uint32_t sample_rate, uint32_t channels, ma_rb_t* rb) {
    ma_dev_t* d = (ma_dev_t*)calloc(1, sizeof(ma_dev_t));
    if (!d) return NULL;
    d->rb = rb;
    d->channels = channels;
    d->volume = 1.0f;
    d->pan = 0.0f;
    ma_device_config config = ma_device_config_init(ma_device_type_playback);
    config.playback.format = ma_format_f32;
    config.playback.channels = channels;
    config.sampleRate = sample_rate;
    config.dataCallback = dev_data_cb;
    config.pUserData = d;
    if (ma_device_init(NULL, &config, &d->device) != MA_SUCCESS) {
        free(d); return NULL;
    }
    return d;
}

bool dev_start(ma_dev_t* d) { return d && ma_device_start(&d->device) == MA_SUCCESS; }
void dev_stop(ma_dev_t* d)  { if (d) ma_device_stop(&d->device); }

void dev_set_volume(ma_dev_t* d, float v) {
    if (!d) return;
    if (v < 0.0f) v = 0.0f;
    if (v > 1.0f) v = 1.0f;
    d->volume = v;
}
float dev_get_volume(ma_dev_t* d) { return d ? d->volume : 0.0f; }

void dev_set_pan(ma_dev_t* d, float p) {
    if (!d) return;
    if (p < -1.0f) p = -1.0f;
    if (p > 1.0f) p = 1.0f;
    d->pan = p;
}

void dev_close(ma_dev_t* d) {
    if (!d) return;
    ma_device_uninit(&d->device);
    free(d);
}
