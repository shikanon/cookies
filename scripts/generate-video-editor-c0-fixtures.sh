#!/bin/sh
set -eu

output_root="${1:-.tmp/video-editor-c0}"
mkdir -p "$output_root"

common_video="-an -c:v libx264 -preset medium -crf 18 -threads 1 -x264-params asm=0 -pix_fmt yuv420p -r 30 -movflags +faststart -map_metadata -1"

# shellcheck disable=SC2086
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=720x1280:rate=30:duration=6" $common_video "$output_root/video-a.mp4"
# shellcheck disable=SC2086
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "smptebars=size=720x1280:rate=30:duration=6" $common_video "$output_root/video-b.mp4"
# shellcheck disable=SC2086
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=1280x720:rate=30:duration=6" $common_video "$output_root/video-landscape.mp4"
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "testsrc2=size=720x1280:rate=30:duration=6" \
  -f lavfi -i "sine=frequency=220:sample_rate=48000:duration=6" \
  -map 0:v:0 -map 1:a:0 -c:v libx264 -preset medium -crf 18 -threads 1 -x264-params asm=0 -pix_fmt yuv420p -r 30 \
  -c:a aac -b:a 128k -ar 48000 -ac 2 -shortest -movflags +faststart -map_metadata -1 \
  "$output_root/video-original-audio.mp4"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "color=c=0x2457d6:size=320x180" -frames:v 1 -map_metadata -1 "$output_root/overlay-opaque.png"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "color=c=0x36c98f@0.55:size=320x180,format=rgba" -frames:v 1 -map_metadata -1 "$output_root/overlay-alpha.png"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "sine=frequency=660:sample_rate=48000:duration=3" -c:a pcm_s16le -map_metadata -1 "$output_root/voice.wav"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=3" -c:a pcm_s16le -map_metadata -1 "$output_root/music.wav"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "sine=frequency=880:sample_rate=48000:duration=1" -c:a pcm_s16le -map_metadata -1 "$output_root/sfx.wav"
printf '1\n00:00:01,000 --> 00:00:05,000\n固定黄金字幕\n' > "$output_root/caption.srt"

font_path="$(fc-match -f '%{file}\n' 'Noto Sans CJK SC' | head -n 1)"
test -n "$font_path"
cp "$font_path" "$output_root/NotoSansCJK-Regular.ttc"

{
  ffmpeg -version
  printf '\n'
  ffprobe -version
  printf '\n'
  fc-match 'Noto Sans CJK SC'
} > "$output_root/render-toolchain.txt"

sha256sum \
  "$output_root/video-a.mp4" \
  "$output_root/video-b.mp4" \
  "$output_root/video-landscape.mp4" \
  "$output_root/video-original-audio.mp4" \
  "$output_root/overlay-opaque.png" \
  "$output_root/overlay-alpha.png" \
  "$output_root/voice.wav" \
  "$output_root/music.wav" \
  "$output_root/sfx.wav" \
  "$output_root/caption.srt" \
  "$output_root/NotoSansCJK-Regular.ttc" > "$output_root/SHA256SUMS"
