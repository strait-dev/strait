import getUnicodeFlagIcon from "country-flag-icons/unicode";
import { getAllISOCodes } from "iso-country-currency";

export const countries = getAllISOCodes().map((country) => ({
  value: country.countryName,
  label: country.countryName,
  iso: country.iso,
  symbol: country.symbol,
  flag: getUnicodeFlagIcon(country.iso),
}));
