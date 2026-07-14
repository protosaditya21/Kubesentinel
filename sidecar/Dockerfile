FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sidecar ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/sidecar /sidecar
USER 65532:65532
ENTRYPOINT ["/sidecar"]
