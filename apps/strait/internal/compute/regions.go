package compute

import "sort"

// IsValidRegion returns true if code is a known Fly region.
func IsValidRegion(code string) bool {
	_, ok := flyRegions[code]
	return ok
}

// AllRegionCodes returns all known Fly region codes in sorted order.
func AllRegionCodes() []string {
	codes := make([]string, 0, len(flyRegions))
	for code := range flyRegions {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// RegionInfo contains metadata about a Fly region.
type RegionInfo struct {
	Code      string `json:"code"`
	Label     string `json:"label"`
	City      string `json:"city"`
	Country   string `json:"country"`
	Continent string `json:"continent"`
}

// RegionLabel returns a human-readable label for a Fly region code.
func RegionLabel(code string) string {
	if info, ok := regionMetadata[code]; ok {
		return info.Label
	}
	return code
}

// AllRegions returns metadata for all known Fly regions, sorted by code.
func AllRegions() []RegionInfo {
	regions := make([]RegionInfo, 0, len(regionMetadata))
	for _, info := range regionMetadata {
		regions = append(regions, info)
	}
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Code < regions[j].Code
	})
	return regions
}

// regionMetadata contains human-readable labels and groupings for all Fly regions.
var regionMetadata = map[string]RegionInfo{
	"ams": {Code: "ams", Label: "Amsterdam, Netherlands", City: "Amsterdam", Country: "NL", Continent: "Europe"},
	"arn": {Code: "arn", Label: "Stockholm, Sweden", City: "Stockholm", Country: "SE", Continent: "Europe"},
	"atl": {Code: "atl", Label: "Atlanta, Georgia (US)", City: "Atlanta", Country: "US", Continent: "North America"},
	"bog": {Code: "bog", Label: "Bogotá, Colombia", City: "Bogotá", Country: "CO", Continent: "South America"},
	"bom": {Code: "bom", Label: "Mumbai, India", City: "Mumbai", Country: "IN", Continent: "Asia"},
	"bos": {Code: "bos", Label: "Boston, Massachusetts (US)", City: "Boston", Country: "US", Continent: "North America"},
	"cdg": {Code: "cdg", Label: "Paris, France", City: "Paris", Country: "FR", Continent: "Europe"},
	"den": {Code: "den", Label: "Denver, Colorado (US)", City: "Denver", Country: "US", Continent: "North America"},
	"dfw": {Code: "dfw", Label: "Dallas, Texas (US)", City: "Dallas", Country: "US", Continent: "North America"},
	"ewr": {Code: "ewr", Label: "Secaucus, NJ (US)", City: "Secaucus", Country: "US", Continent: "North America"},
	"eze": {Code: "eze", Label: "Buenos Aires, Argentina", City: "Buenos Aires", Country: "AR", Continent: "South America"},
	"fra": {Code: "fra", Label: "Frankfurt, Germany", City: "Frankfurt", Country: "DE", Continent: "Europe"},
	"gdl": {Code: "gdl", Label: "Guadalajara, Mexico", City: "Guadalajara", Country: "MX", Continent: "North America"},
	"gig": {Code: "gig", Label: "Rio de Janeiro, Brazil", City: "Rio de Janeiro", Country: "BR", Continent: "South America"},
	"gru": {Code: "gru", Label: "São Paulo, Brazil", City: "São Paulo", Country: "BR", Continent: "South America"},
	"hkg": {Code: "hkg", Label: "Hong Kong", City: "Hong Kong", Country: "HK", Continent: "Asia"},
	"iad": {Code: "iad", Label: "Ashburn, Virginia (US)", City: "Ashburn", Country: "US", Continent: "North America"},
	"icn": {Code: "icn", Label: "Seoul, South Korea", City: "Seoul", Country: "KR", Continent: "Asia"},
	"jnb": {Code: "jnb", Label: "Johannesburg, South Africa", City: "Johannesburg", Country: "ZA", Continent: "Africa"},
	"lax": {Code: "lax", Label: "Los Angeles, California (US)", City: "Los Angeles", Country: "US", Continent: "North America"},
	"lhr": {Code: "lhr", Label: "London, United Kingdom", City: "London", Country: "GB", Continent: "Europe"},
	"mad": {Code: "mad", Label: "Madrid, Spain", City: "Madrid", Country: "ES", Continent: "Europe"},
	"mia": {Code: "mia", Label: "Miami, Florida (US)", City: "Miami", Country: "US", Continent: "North America"},
	"nrt": {Code: "nrt", Label: "Tokyo, Japan", City: "Tokyo", Country: "JP", Continent: "Asia"},
	"ord": {Code: "ord", Label: "Chicago, Illinois (US)", City: "Chicago", Country: "US", Continent: "North America"},
	"otp": {Code: "otp", Label: "Bucharest, Romania", City: "Bucharest", Country: "RO", Continent: "Europe"},
	"phx": {Code: "phx", Label: "Phoenix, Arizona (US)", City: "Phoenix", Country: "US", Continent: "North America"},
	"qro": {Code: "qro", Label: "Querétaro, Mexico", City: "Querétaro", Country: "MX", Continent: "North America"},
	"scl": {Code: "scl", Label: "Santiago, Chile", City: "Santiago", Country: "CL", Continent: "South America"},
	"sea": {Code: "sea", Label: "Seattle, Washington (US)", City: "Seattle", Country: "US", Continent: "North America"},
	"sin": {Code: "sin", Label: "Singapore", City: "Singapore", Country: "SG", Continent: "Asia"},
	"sjc": {Code: "sjc", Label: "San Jose, California (US)", City: "San Jose", Country: "US", Continent: "North America"},
	"syd": {Code: "syd", Label: "Sydney, Australia", City: "Sydney", Country: "AU", Continent: "Oceania"},
	"waw": {Code: "waw", Label: "Warsaw, Poland", City: "Warsaw", Country: "PL", Continent: "Europe"},
	"yul": {Code: "yul", Label: "Montreal, Canada", City: "Montreal", Country: "CA", Continent: "North America"},
	"yyz": {Code: "yyz", Label: "Toronto, Canada", City: "Toronto", Country: "CA", Continent: "North America"},
}

// NearestFlyRegion maps region hints (from Fly-Region header or continent
// hints) to Fly region codes. If the input is already a valid Fly region
// code it is returned as-is.
func NearestFlyRegion(hint string) string {
	// Direct Fly region codes pass through.
	if _, ok := flyRegions[hint]; ok {
		return hint
	}

	// Continent/zone hints.
	if region, ok := regionHints[hint]; ok {
		return region
	}

	return ""
}

// RegionFallbackChain returns up to 3 geo-proximate alternative regions.
// Returns nil for unknown regions.
func RegionFallbackChain(primary string) []string {
	chain, ok := regionFallbacks[primary]
	if !ok {
		return nil
	}
	return chain
}

// regionFallbacks maps each Fly region to geo-proximate alternatives.
var regionFallbacks = map[string][]string{
	// North America East
	"iad": {"ewr", "ord", "atl"},
	"ewr": {"iad", "bos", "ord"},
	"atl": {"iad", "mia", "ord"},
	"bos": {"ewr", "iad", "yul"},
	"mia": {"atl", "iad", "bog"},
	"ord": {"iad", "den", "dfw"},
	"yul": {"ewr", "bos", "iad"},
	"yyz": {"ewr", "ord", "bos"},

	// North America West
	"lax": {"sjc", "sea", "phx"},
	"sjc": {"lax", "sea", "phx"},
	"sea": {"sjc", "lax", "den"},
	"den": {"ord", "sea", "dfw"},
	"phx": {"lax", "den", "dfw"},
	"dfw": {"ord", "den", "atl"},

	// Europe
	"lhr": {"cdg", "fra", "ams"},
	"cdg": {"lhr", "fra", "ams"},
	"fra": {"cdg", "ams", "waw"},
	"ams": {"lhr", "cdg", "fra"},
	"arn": {"fra", "ams", "waw"},
	"waw": {"fra", "arn", "otp"},
	"mad": {"cdg", "lhr", "fra"},
	"otp": {"waw", "fra", "arn"},

	// Asia
	"nrt": {"hkg", "sin", "icn"},
	"hkg": {"nrt", "sin", "bom"},
	"sin": {"hkg", "nrt", "bom"},
	"bom": {"sin", "hkg", "nrt"},
	"icn": {"nrt", "hkg", "sin"},

	// Oceania
	"syd": {"sin", "nrt", "hkg"},

	// Latin America
	"gru": {"gig", "eze", "scl"},
	"gig": {"gru", "eze", "bog"},
	"eze": {"gru", "scl", "gig"},
	"scl": {"eze", "gru", "bog"},
	"bog": {"mia", "gru", "gdl"},
	"gdl": {"dfw", "lax", "qro"},
	"qro": {"gdl", "dfw", "lax"},

	// Africa
	"jnb": {"lhr", "cdg", "fra"},
}

// regionHints maps continent-level hints to a default Fly region.
var regionHints = map[string]string{
	"us-east": "iad",
	"us-west": "lax",
	"eu":      "lhr",
	"europe":  "lhr",
	"asia":    "nrt",
	"oceania": "syd",
	"sa":      "gru",
	"africa":  "jnb",
}

// flyRegions is the set of known Fly region codes.
var flyRegions = map[string]struct{}{
	"ams": {}, "arn": {}, "atl": {}, "bog": {}, "bom": {},
	"bos": {}, "cdg": {}, "den": {}, "dfw": {}, "ewr": {},
	"eze": {}, "fra": {}, "gdl": {}, "gig": {}, "gru": {},
	"hkg": {}, "iad": {}, "icn": {}, "jnb": {}, "lax": {}, "lhr": {},
	"mad": {}, "mia": {}, "nrt": {}, "ord": {}, "otp": {},
	"phx": {}, "qro": {}, "scl": {}, "sea": {}, "sin": {},
	"sjc": {}, "syd": {}, "waw": {}, "yul": {}, "yyz": {},
}
