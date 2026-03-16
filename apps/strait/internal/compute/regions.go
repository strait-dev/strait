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
