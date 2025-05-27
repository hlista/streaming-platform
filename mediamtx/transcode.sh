#!/bin/sh
STREAM_KEY=$1
MTX_PATH=$2

# Send webhook notification in background
curl -X POST http://webhook-handler-1:8080/webhooks/stream/start \
  -H "Content-Type: application/json" \
  -d "{\"stream_key\":\"${STREAM_KEY}\",\"path\":\"${MTX_PATH}\"}" &

# Start FFmpeg transcoding
exec ffmpeg -i rtmp://localhost:1935/${MTX_PATH} \
  -c:v libx264 -preset veryfast -tune zerolatency -g 30 \
  -map 0:v:0 -map 0:a:0 \
  -s:v:0 1920x1080 -b:v:0 5000k -maxrate:v:0 5500k -bufsize:v:0 10000k \
  -s:v:1 1280x720 -b:v:1 2500k -maxrate:v:1 2750k -bufsize:v:1 5000k \
  -s:v:2 854x480 -b:v:2 1000k -maxrate:v:2 1100k -bufsize:v:2 2000k \
  -s:v:3 640x360 -b:v:3 500k -maxrate:v:3 550k -bufsize:v:3 1000k \
  -c:a aac -b:a 128k -ac 2 \
  -var_stream_map "v:0,a:0,name:1080p v:1,a:1,name:720p v:2,a:2,name:480p v:3,a:3,name:360p" \
  -f hls -hls_time 1 -hls_list_size 5 -hls_flags delete_segments \
  -master_pl_name master.m3u8 \
  -hls_segment_filename /hls/${STREAM_KEY}_%v_%03d.ts \
  /hls/${STREAM_KEY}_%v.m3u8
  