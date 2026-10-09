FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /traffic-visualizer-api .

FROM alpine:3.22
# ca-certificates: the backend downloads the GeoIP databases over HTTPS.
# /data is where they live; a new named volume mounted there inherits this owner.
RUN apk add --no-cache traceroute ca-certificates && adduser -D -u 10001 app && mkdir /data && chown app /data
USER app
COPY --from=build /traffic-visualizer-api /usr/local/bin/traffic-visualizer-api
ENV GEOIP_CITY_DB=/data/GeoLite2-City.mmdb GEOIP_ASN_DB=/data/GeoLite2-ASN.mmdb GEOIP_AUTO_UPDATE=true
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["traffic-visualizer-api"]
