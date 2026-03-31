# Build stage
FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /faultline ./cmd/faultline

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /faultline /faultline

EXPOSE 8080

ENTRYPOINT ["/faultline"]
