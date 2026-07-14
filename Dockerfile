FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/manager .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
