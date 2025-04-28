FROM golang:1.24.2-alpine AS build
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /src
COPY . .
RUN go mod download
RUN go build -o /inferno

FROM registry.access.redhat.com/ubi8/ubi-minimal

WORKDIR /
COPY --from=build /inferno /inferno

ENTRYPOINT ["/inferno"]
