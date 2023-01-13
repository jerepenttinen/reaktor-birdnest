FROM golang:1.19-buster AS build

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o /reaktor-birdnest ./cmd/api

FROM gcr.io/distroless/base-debian11

COPY --from=build /reaktor-birdnest /reaktor-birdnest

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/reaktor-birdnest"]
