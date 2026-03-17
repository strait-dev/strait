package compute

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
	"hkg": {}, "iad": {}, "jnb": {}, "lax": {}, "lhr": {},
	"mad": {}, "mia": {}, "nrt": {}, "ord": {}, "otp": {},
	"phx": {}, "qro": {}, "scl": {}, "sea": {}, "sin": {},
	"sjc": {}, "syd": {}, "waw": {}, "yul": {}, "yyz": {},
}
