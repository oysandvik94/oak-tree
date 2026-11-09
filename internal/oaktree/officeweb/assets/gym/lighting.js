const bergen = {latitude: 60.39299, longitude: 5.32415};
const radians = Math.PI / 180;

// NOAA's approximate solar position, evaluated in UTC to avoid timezone/DST jumps.
// https://gml.noaa.gov/grad/solcalc/solareqns.PDF
export function solarElevationAt(date, {latitude, longitude} = bergen) {
  const year = date.getUTCFullYear();
  const start = Date.UTC(year, 0, 1);
  const daysInYear = (Date.UTC(year + 1, 0, 1) - start) / 86400000;
  const day = (date.getTime() - start) / 86400000;
  const angle = 2 * Math.PI / daysInYear * (day - .5);
  const equationOfTime = 229.18 * (.000075 + .001868 * Math.cos(angle)
    - .032077 * Math.sin(angle) - .014615 * Math.cos(2 * angle) - .040849 * Math.sin(2 * angle));
  const declination = .006918 - .399912 * Math.cos(angle) + .070257 * Math.sin(angle)
    - .006758 * Math.cos(2 * angle) + .000907 * Math.sin(2 * angle)
    - .002697 * Math.cos(3 * angle) + .00148 * Math.sin(3 * angle);
  const minutes = date.getUTCHours() * 60 + date.getUTCMinutes() + date.getUTCSeconds() / 60;
  const hourAngle = ((minutes + equationOfTime + 4 * longitude) / 4 - 180) * radians;
  const latitudeAngle = latitude * radians;
  const sine = Math.sin(latitudeAngle) * Math.sin(declination)
    + Math.cos(latitudeAngle) * Math.cos(declination) * Math.cos(hourAngle);
  return Math.asin(Math.max(-1, Math.min(1, sine))) / radians;
}

export function daylightAt(date = new Date(), location = bergen) {
  // Fade through civil twilight into low-angle daylight; no sunrise event is
  // required, so polar night and midnight sun work without special fallbacks.
  const t = Math.max(0, Math.min(1, (solarElevationAt(date, location) + 6) / 12));
  return t * t * (3 - 2 * t);
}
