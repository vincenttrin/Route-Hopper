// Example response shown on first load, shaped like POST /api/trace. Like a real response it starts
// with the hidden hop that stands in for the source's own network.
export const SAMPLE_ENDPOINT = 'example.com';

const h = (hopNumber, ip, hostname, city, lat, lng, rtt, org) => ({ hopNumber, ip, hostname, city, lat, lng, rtt, org });
const HIDDEN_START = { hopNumber: 1, hidden: true };

export const SAMPLE_TRACE = {
  hops: [
    HIDDEN_START,
    h(2, '68.1.1.37', 'chgil-cr1.cox.net', 'Chicago', 41.88, -87.63, 19.2, 'Cox'),
    h(3, '4.68.62.9', 'ae7.cr1.ord1.lumen.net', 'Chicago', 41.88, -87.63, 21.0, 'Lumen'),
    h(4, '4.69.201.6', 'ae2.cr2.iad1.lumen.net', 'Ashburn', 39.04, -77.49, 34.7, 'Lumen'),
    h(5, '4.68.111.130', 'nyc-b2.lumen.net', 'New York', 40.71, -74.0, 41.3, 'Lumen'),
    h(6, '', '', '', 0, 0, 0),
    h(7, '213.200.80.1', 'ae-12.dub.gtt.net', 'Dublin', 53.35, -6.26, 112.9, 'GTT'),
    h(8, '213.200.80.6', 'ae-3.lon.gtt.net', 'London', 51.51, -0.13, 121.4, 'GTT'),
    h(9, '93.184.216.34', 'example.com', 'Amsterdam', 52.37, 4.9, 128.0, 'Edgecast'),
  ],
  destination: { ip: '93.184.216.34', lat: 52.37, lng: 4.9 },
};
