# syntax=docker/dockerfile:1

# Build stage. Keep the Go minor version in step with go.mod.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/setlisted ./cmd/setlisted

# Final stage: a single static binary on distroless, running as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/setlisted /setlisted
# Cloud Run sets PORT. The app defaults to 8080 when it's unset.
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/setlisted"]
