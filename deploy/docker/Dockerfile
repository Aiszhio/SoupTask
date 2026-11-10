FROM golang:1.27.3-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/SoupWebApplication ./cmd/SoupWebApplication
RUN CGO_ENABLED=0 go build -trimpath -o /out/SoupWorker ./cmd/SoupWorker

FROM scratch
COPY --from=build /out/SoupWebApplication /SoupWebApplication
COPY --from=build /out/SoupWorker /SoupWorker
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/SoupWebApplication"]