# Stage 1: generate ontology artefacts via the project Makefile
FROM python:3.12-slim AS docs-builder
RUN apt-get update && apt-get install -y --no-install-recommends make && rm -rf /var/lib/apt/lists/*
WORKDIR /repo
COPY docs/ docs/
COPY pdf-core/Makefile pdf-core/Makefile
RUN make -C pdf-core docs

# Stage 2: build the Go binary
FROM golang:1.26.10-alpine AS builder
WORKDIR /app
COPY pdf-core/go.mod pdf-core/go.sum ./
RUN go mod download
COPY pdf-core/ .
COPY --from=docs-builder /repo/pdf-core/docs/ docs/
RUN CGO_ENABLED=0 go build -o dcspdfcore .

FROM alpine:3.20
# Fixed numeric UID/GID: the Helm chart runs the pod as this user with
# runAsNonRoot (values.yaml, pdfCore.podSecurityContext).
RUN addgroup -S -g 10001 app && adduser -S -u 10001 -G app app
WORKDIR /app
COPY --from=builder /app/dcspdfcore ./dcspdfcore
USER 10001:10001
EXPOSE 8080
ENV DCS_PDF_CORE_ADDR=0.0.0.0:8080
ENTRYPOINT ["./dcspdfcore"]
