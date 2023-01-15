FROM golang:1.19-buster AS build

WORKDIR /app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 go build -o /reaktor-birdnest -installsuffix "static" ./cmd/api

FROM gcr.io/distroless/static-debian11

COPY --from=build /reaktor-birdnest /reaktor-birdnest

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/reaktor-birdnest"]
