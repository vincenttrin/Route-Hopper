// Example response shown on first load, shaped like POST /api/trace.
export const SAMPLE_ENDPOINT = 'example.com';

const h = (hopNumber, ip, hostname, city, lat, lng, rtt, org) => ({ hopNumber, ip, hostname, city, lat, lng, rtt, org });

export const SAMPLE_TRACE = {
  hops: [
    h(1, '192.168.1.1', 'gateway.lan', 'Omaha', 41.26, -95.93, 0.9),
    h(2, '192.0.2.1', 'ae-1.rtr.omaha.telecom.example', 'Omaha', 41.25, -95.94, 8.4, 'Example Telecom'),
    h(3, '192.0.2.37', 'cr1.chi.telecom.example', 'Chicago', 41.88, -87.63, 19.2, 'Example Telecom'),
    h(4, '198.51.100.9', 'cr1.chi.backbone.example', 'Chicago', 41.88, -87.63, 21.0, 'Example Backbone'),
    h(5, '198.51.100.6', 'cr2.iad.backbone.example', 'Ashburn', 39.04, -77.49, 34.7, 'Example Backbone'),
    h(6, '198.51.100.130', 'cr1.nyc.backbone.example', 'New York', 40.71, -74.0, 41.3, 'Example Backbone'),
    h(7, '', '', '', 0, 0, 0),
    h(8, '203.0.113.81', 'ae-12.dub.transit.example', 'Dublin', 53.35, -6.26, 112.9, 'Example Transit'),
    h(9, '203.0.113.86', 'ae-3.lon.transit.example', 'London', 51.51, -0.13, 121.4, 'Example Transit'),
    h(10, '203.0.113.10', 'example.com', 'Amsterdam', 52.37, 4.9, 128.0, 'Example CDN'),
  ],
  destination: { ip: '203.0.113.10', lat: 52.37, lng: 4.9 },
};
