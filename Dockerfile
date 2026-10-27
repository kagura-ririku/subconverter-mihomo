FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24 AS builder

WORKDIR /src

COPY go.mod go.sum ./
ARG GOPROXY
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/subconverter ./cmd/subconverter

FROM alpine:3.24.3

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/subconverter /usr/local/bin/subconverter

ENTRYPOINT ["subconverter"]
