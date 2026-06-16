FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/app ./cmd
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/app /app/app
COPY config.yaml /app/config.yaml
EXPOSE 8080
ENTRYPOINT ["/app/app"]
