# build stage
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/engine ./cmd/engine

# run stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/engine /engine
EXPOSE 13307
ENTRYPOINT ["/engine"]
CMD ["-addr", ":13307"]
