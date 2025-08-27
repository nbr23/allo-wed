FROM --platform=${BUILDOS}/${BUILDARCH} golang:alpine AS builder

WORKDIR /build

COPY go* *.go /build/

RUN GOOS=linux GOARCH=arm64 go build -trimpath -o allo-wed-linux-arm64
RUN GOOS=linux GOARCH=amd64 go build -trimpath -o allo-wed-linux-amd64

# Base

FROM --platform=${TARGETOS}/${TARGETARCH} alpine:latest AS base
ARG TARGETARCH
ARG TARGETOS

COPY --from=builder /build/allo-wed-${TARGETOS}-${TARGETARCH} /usr/bin/allo-wed

ENTRYPOINT ["allo-wed"]

# Asterisk
FROM --platform=${TARGETOS}/${TARGETARCH} nbr23/asterisk:latest AS asterisk
ARG TARGETARCH
ARG TARGETOS

COPY --from=builder /build/allo-wed-${TARGETOS}-${TARGETARCH} /usr/bin/allo-wed
