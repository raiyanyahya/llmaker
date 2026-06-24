package engine

import (
	"math/rand/v2"
	"strings"
)

// When the user doesn't name an instance, we generate a friendly, memorable
// adjective-animal handle (e.g. "brave-llama") instead of an opaque hash, so
// `llmaker ls` reads nicely and names are easy to type.
var (
	nameAdjectives = []string{
		"brave", "calm", "clever", "cosmic", "eager", "fuzzy", "gentle",
		"happy", "jolly", "keen", "lucky", "mellow", "nimble", "plucky",
		"quiet", "rapid", "sleek", "snappy", "spry", "swift", "witty", "zesty",
	}
	nameAnimals = []string{
		"llama", "alpaca", "vicuna", "otter", "falcon", "lynx", "marmot",
		"narwhal", "octopus", "panther", "quokka", "raven", "stingray",
		"tapir", "urchin", "viper", "walrus", "yak", "zebra", "badger",
	}
)

// GenerateName returns a random friendly instance name.
func GenerateName() string {
	return nameFrom(rand.IntN(len(nameAdjectives)), rand.IntN(len(nameAnimals)))
}

// GenerateUniqueName returns a friendly name not present in taken, falling back
// to a numeric suffix if the namespace is saturated.
func GenerateUniqueName(taken map[string]bool) string {
	for attempt := 0; attempt < 64; attempt++ {
		n := GenerateName()
		if !taken[n] {
			return n
		}
	}
	// Extremely unlikely; disambiguate deterministically.
	base := GenerateName()
	for i := 2; ; i++ {
		candidate := base + "-" + itoa(i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// nameFrom is the deterministic core of name generation, split out for testing.
func nameFrom(adj, animal int) string {
	a := nameAdjectives[((adj%len(nameAdjectives))+len(nameAdjectives))%len(nameAdjectives)]
	n := nameAnimals[((animal%len(nameAnimals))+len(nameAnimals))%len(nameAnimals)]
	return a + "-" + n
}

// ValidName reports whether s is acceptable as an instance name. We keep it to
// DNS-ish characters so it composes cleanly into container and volume names.
func ValidName(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9':
			// always allowed
		case r == '-' || r == '_':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// NormalizeName lowercases and trims a user-supplied name for consistency.
func NormalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
