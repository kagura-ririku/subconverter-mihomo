FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine3.24 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/subconverter ./cmd/subconverter

FROM alpine:3.24.1

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/subconverter /usr/local/bin/subconverter

ENTRYPOINT ["subconverter"]
