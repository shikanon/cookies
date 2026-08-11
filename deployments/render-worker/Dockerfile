ARG GO_IMAGE=golang@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2
ARG ALPINE_IMAGE=alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

FROM ${GO_IMAGE} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cookies-api ./cmd/cookies-api

FROM ${GO_IMAGE} AS verifier
RUN apk add --no-cache \
      ffmpeg=8.1.2-r0 \
      font-noto-cjk=0_git20220127-r1 \
      fontconfig=2.17.1-r1
WORKDIR /workspace
CMD ["sh"]

FROM ${ALPINE_IMAGE} AS runtime
LABEL org.opencontainers.image.title="cookies video render worker" \
      org.opencontainers.image.description="Pinned FFmpeg toolchain for cookies timeline rendering" \
      io.cookies.ffmpeg.package="8.1.2-r0" \
      io.cookies.font.noto-cjk.package="0_git20220127-r1"
RUN apk add --no-cache \
      ffmpeg=8.1.2-r0 \
      font-noto-cjk=0_git20220127-r1 \
      fontconfig=2.17.1-r1 \
      ca-certificates=20260611-r0 && \
    mkdir -p /opt/cookies /var/lib/cookies && \
    { ffmpeg -version; printf '\n'; ffprobe -version; printf '\n'; fc-match 'Noto Sans CJK SC'; } > /opt/cookies/render-toolchain.txt
COPY --from=builder /out/cookies-api /usr/local/bin/cookies-api
ENV COOKIES_FFMPEG_PATH=/usr/bin/ffmpeg \
    COOKIES_FFPROBE_PATH=/usr/bin/ffprobe
VOLUME ["/var/lib/cookies"]
ENTRYPOINT ["/usr/local/bin/cookies-api"]
