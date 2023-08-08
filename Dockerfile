ARG BASE_VERSION=latest
FROM golang:1.20 as build

WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -o /go/bin/rrip

FROM gcr.io/distroless/static-debian11:${BASE_VERSION}
COPY --from=build /go/bin/rrip /
WORKDIR /app/
ENTRYPOINT ["/rrip"]

## Run example
## docker run -v $PWD:/app -u $(id -u):$(id -g) rrip --max-size 1000 r/LogicGateMemes
