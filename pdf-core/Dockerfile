# Stage 1: generate ontology artefacts via the project Makefile
FROM python:3.12-slim AS docs-builder
RUN apt-get update && apt-get install -y --no-install-recommends make && rm -rf /var/lib/apt/lists/*
WORKDIR /repo
COPY docs/ docs/
COPY pdf-core/Makefile pdf-core/Makefile
RUN make -C pdf-core docs

# Stage 2: build the Go binary
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY pdf-core/go.mod pdf-core/go.sum ./
RUN go mod download
COPY pdf-core/ .
COPY --from=docs-builder /repo/pdf-core/docs/ docs/
RUN CGO_ENABLED=0 go build -o dcspdfcore .

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /app/dcspdfcore ./dcspdfcore
USER app
EXPOSE 8080
ENV DCS_PDF_CORE_ADDR=0.0.0.0:8080
ENTRYPOINT ["./dcspdfcore"]
