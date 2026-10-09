FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /traffic-visualizer-api .

FROM alpine:3.22
RUN apk add --no-cache traceroute && adduser -D -u 10001 app
USER app
COPY --from=build /traffic-visualizer-api /usr/local/bin/traffic-visualizer-api
ENV GEOIP_CITY_DB=/data/GeoLite2-City.mmdb GEOIP_ASN_DB=/data/GeoLite2-ASN.mmdb
EXPOSE 8080
ENTRYPOINT ["traffic-visualizer-api"]
