#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

#include <linux/videodev2.h>

#include "parameters.h"

void parameters_load(parameters_t *params) {
    params->camera_id = atoi(getenv("CAMERA_ID"));
    params->width = atoi(getenv("WIDTH"));
    params->height = atoi(getenv("HEIGHT"));
    params->h_flip = (strcmp(getenv("H_FLIP"), "1") == 0);
    params->v_flip = (strcmp(getenv("V_FLIP"), "1") == 0);
    params->brightness = atof(getenv("BRIGHTNESS"));
    params->contrast = atof(getenv("CONTRAST"));
    params->saturation = atof(getenv("SATURATION"));
    params->sharpness = atof(getenv("SHARPNESS"));
    params->exposure = getenv("EXPOSURE");
    params->awb = getenv("AWB");
    params->denoise = getenv("DENOISE");
    params->shutter = atoi(getenv("SHUTTER"));
    params->metering = getenv("METERING");
    params->gain = atof(getenv("GAIN"));
    params->ev = atof(getenv("EV"));
    params->roi = getenv("ROI");
    params->tuning_file = getenv("TUNING_FILE");
    params->fps = atoi(getenv("FPS"));
    params->idr_period = atoi(getenv("IDR_PERIOD"));
    params->bitrate = atoi(getenv("BITRATE"));

    const char *profile = getenv("PROFILE");
    if (strcmp(profile, "baseline") == 0) {
        params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_BASELINE;
    } else if (strcmp(profile, "main") == 0) {
        params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_MAIN;
    } else {
        params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_HIGH;
    }

    const char *level = getenv("LEVEL");
    if (strcmp(level, "4.0") == 0) {
        params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_0;
    } else if (strcmp(level, "4.1") == 0) {
        params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_1;
    } else {
        params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_2;
    }

    params->buffer_count = 3;
    params->capture_buffer_count = params->buffer_count * 2;
}
