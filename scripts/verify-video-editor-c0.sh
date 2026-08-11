#!/bin/sh
set -eu

fixture_root="${COOKIES_C0_FIXTURE_ROOT:-.tmp/video-editor-c0}"
sh scripts/generate-video-editor-c0-fixtures.sh "$fixture_root"
fixture_root="$(cd "$fixture_root" && pwd)"
export COOKIES_C0_FIXTURE_ROOT="$fixture_root"

export COOKIES_REQUIRE_TIMELINE_GOLDEN=1
export COOKIES_TEST_FFMPEG_PATH="$(command -v ffmpeg)"
export COOKIES_TEST_FFPROBE_PATH="$(command -v ffprobe)"
export COOKIES_TEST_TIMELINE_VIDEO_A="$fixture_root/video-a.mp4"
export COOKIES_TEST_TIMELINE_VIDEO_B="$fixture_root/video-b.mp4"
export COOKIES_TEST_TIMELINE_AUDIO="$fixture_root/music.wav"

go test ./internal/platform/media -run 'TestFFmpegTimelinePreviewAndExportMatchGoldenMedia|TestFFmpegC3VisualLayersMatchFixedGolden|TestFFmpegC7FrozenMultitrackPreviewAndExportMatchGolden' -count=1
go test ./internal/platform/assets -run TestC0FixtureIsPreviewableThroughAssetService -count=1
