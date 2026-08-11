# Video render toolchain

This image is the C0 baseline for authoritative video-editor preview and export.
It builds the existing `cookies-api` binary together with a pinned FFmpeg and
font stack. The API continues to own RenderJob scheduling; the image freezes
the executable environment used by that scheduler.

## Frozen inputs

- Go builder: `golang@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2`
- Runtime: `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`
- FFmpeg: Alpine package `ffmpeg=8.1.2-r0`
- Fontconfig: Alpine package `fontconfig=2.17.1-r1`
- Chinese font fixture: Alpine package `font-noto-cjk=0_git20220127-r1`

The runtime image writes complete `ffmpeg -version`, `ffprobe -version`,
configure flags, and the resolved Chinese font to
`/opt/cookies/render-toolchain.txt`. Package labels are also stored in the OCI
image metadata.

Build the runtime image:

```sh
docker build --target runtime -f deployments/render-worker/Dockerfile -t cookies-render-worker:c0 .
```

Inspect the frozen build record:

```sh
docker run --rm --entrypoint cat cookies-render-worker:c0 /opt/cookies/render-toolchain.txt
```

## License boundary

The image uses Alpine's FFmpeg package, including the H.264 encoder used by the
current renderer. Before publishing the image outside the organization, retain
the Alpine package source offer and notices and complete the repository's
FFmpeg distribution review. The C0 fixture media itself contains only generated
test patterns, geometric shapes, sine waves, and project-authored subtitle text;
it contains no third-party footage or music.
