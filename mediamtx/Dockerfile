FROM bluenviron/mediamtx:latest-ffmpeg

# Install curl for webhooks
RUN apk add --no-cache curl

# Copy custom scripts
COPY transcode.sh /transcode.sh
RUN chmod +x /transcode.sh