package discovery

// SecretPattern defines a naming pattern that identifies a secret.
type SecretPattern struct {
	Suffix string // e.g. "_PASSWORD"
	Type   string // e.g. "password"
}

// DefaultPatterns are suffix-based patterns for identifying secrets in env var names.
var DefaultPatterns = []SecretPattern{
	{Suffix: "_API_KEY", Type: "api_key"},
	{Suffix: "_APIKEY", Type: "api_key"},
	{Suffix: "_PASSWORD", Type: "password"},
	{Suffix: "_PASSWD", Type: "password"},
	{Suffix: "_SECRET", Type: "secret"},
	{Suffix: "_TOKEN", Type: "token"},
	{Suffix: "_KEY", Type: "api_key"},
	{Suffix: "_AUTH", Type: "auth"},
	{Suffix: "_CREDENTIAL", Type: "credential"},
	{Suffix: "_PRIVATE_KEY", Type: "private_key"},
}

// ExactPatterns are exact key names that identify secrets.
var ExactPatterns = map[string]string{
	"SECRET_KEY":     "secret",
	"JWT_SECRET":     "secret",
	"SESSION_SECRET": "secret",
	"DATABASE_URL":   "connection_string",
	"REDIS_URL":      "connection_string",
	"MONGO_URI":      "connection_string",
}

// CommonDefaults is a set of known weak/default passwords that should always
// be flagged. Keys are lowercase for case-insensitive matching.
var CommonDefaults = map[string]bool{
	"changeme":       true,
	"password":       true,
	"password1":      true,
	"password123":    true,
	"secret":         true,
	"admin":          true,
	"root":           true,
	"default":        true,
	"test":           true,
	"example":        true,
	"12345678":       true,
	"123456789":      true,
	"1234567890":     true,
	"qwerty":         true,
	"letmein":        true,
	"welcome":        true,
	"monkey":         true,
	"master":         true,
	"dragon":         true,
	"login":          true,
	"abc123":         true,
	"passw0rd":       true,
	"p@ssw0rd":       true,
	"p@ssword":       true,
	"trustno1":       true,
	"iloveyou":       true,
	"sunshine":       true,
	"princess":       true,
	"football":       true,
	"shadow":         true,
}
