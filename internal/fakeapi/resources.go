package fakeapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	wordsShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	secretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	runnerKey  = regexp.MustCompile(`^[A-Za-z0-9._~-]{32,255}$`)
	rule       = regexp.MustCompile(`(?s)^([a-z]+)(?:\((.*)\))?$`)
	envName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	bare       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	awsSecret  = regexp.MustCompile(`^[A-Za-z0-9/_+=.@:-]+$`)
	hostShape  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9a-z-]{1,128}$")
	queryName  = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,128}$`)
	prefixText = regexp.MustCompile(`^[\x20-\x7e]{0,64}$`)
	neverWorn  = []string{"host", "content-length", "content-type", "transfer-encoding", "connection", "upgrade", "te", "trailer"}
	harnesses  = map[string]string{"claude-code": "json", "cursor": "json", "codex": "toml"}
)

const longestWords = 64

func words(text string) bool {
	return wordsShape.MatchString(text) && len(text) <= longestWords
}

func wordsLike(example string) func(string) string {
	return func(text string) string {
		if words(text) {
			return ""
		}

		return fmt.Sprintf("is lowercase words joined by dashes, at most %d characters, as in %s", longestWords, example)
	}
}

func onceEach(items []any, at string, wrong func(string) string, errs *[]fieldError) []any {
	seen := map[string]bool{}
	out := make([]any, 0, len(items))

	for index, each := range items {
		text := strings.TrimSpace(textOf(each))
		if why := wrong(text); why != "" {
			*errs = append(*errs, fieldError{fmt.Sprintf("%s[%d]", at, index), why})
		}
		if seen[text] {
			*errs = append(*errs, fieldError{fmt.Sprintf("%s[%d]", at, index), fmt.Sprintf("gives %s a second time", text)})
		}
		seen[text] = true
		out = append(out, text)
	}

	return out
}

func keyed(at, key string) string {
	if bare.MatchString(key) {
		return at + "." + key
	}

	quoted, _ := json.Marshal(key)

	return at + "[" + string(quoted) + "]"
}

func secret(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	name, _ := body["name"].(string)
	if !secretName.MatchString(name) {
		errs = append(errs, fieldError{"name", "is upper case letters, digits and underscores, as in LINEAR_API_KEY"})
	}

	given, _ := body["source"].(map[string]any)
	source := map[string]any{"kind": "built-in", "secretId": nil, "field": nil}
	if kind, _ := given["kind"].(string); kind == "aws-secrets-manager" {
		source["kind"] = kind
		secretID, _ := given["secretId"].(string)
		secretID = strings.TrimSpace(secretID)
		if !awsSecret.MatchString(secretID) || len(secretID) > 2048 {
			errs = append(errs, fieldError{"source.secretId", "is the name or ARN of the secret in AWS Secrets Manager, as in arn:aws:secretsmanager:eu-west-1:123456789012:secret:linear-api-key"})
		}
		source["secretId"] = secretID
		if field, _ := given["field"].(string); field != "" {
			source["field"] = field
		}
	} else if kind != "" && kind != "built-in" {
		errs = append(errs, fieldError{"source.kind", "is built-in or aws-secrets-manager"})
	}

	listed, _ := body["bindings"].([]any)
	if len(listed) > 32 {
		errs = append(errs, fieldError{"bindings", "names more than 32 hosts; a secret goes to the few hosts that accept it"})
	}
	bindings := make([]any, 0, len(listed))
	hosts := map[string]bool{}
	for at, each := range listed {
		binding, _ := each.(map[string]any)
		host, _ := binding["host"].(string)
		host = strings.ToLower(strings.TrimSpace(host))
		carrier, ok := binding["carrier"].(string)
		if !ok {
			carrier = "header"
		}
		name, named := binding["name"].(string)
		prefix, _ := binding["prefix"].(string)

		if !hostShape.MatchString(host) || len(host) > 253 {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].host", at), "is the one host the secret may be sent to, as in api.linear.app, with no scheme, port or path"})
		}
		switch carrier {
		case "header":
			if !named {
				name = "authorization"
			}
			name = strings.ToLower(strings.TrimSpace(name))
			if !headerName.MatchString(name) || slices.Contains(neverWorn, name) {
				errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].name", at), "is the request header the secret is worn in, as in authorization or x-api-key"})
			}
		case "query":
			name = strings.TrimSpace(name)
			if !queryName.MatchString(name) {
				errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].name", at), "is the query parameter the secret is worn in, as in key or api_key"})
			}
		default:
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].carrier", at), "is where the secret is worn: header or query"})
		}
		if !prefixText.MatchString(prefix) {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].prefix", at), `is up to 64 printable characters written before the value, as in "Bearer "`})
		}
		if hosts[host] {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].host", at), fmt.Sprintf("binds %s twice; a secret is worn one way toward each host", host)})
		}
		hosts[host] = true

		bindings = append(bindings, map[string]any{"host": host, "carrier": carrier, "name": name, "prefix": prefix})
	}

	var version any
	if number, ok := body["valueWoVersion"].(float64); ok {
		version = number
	}

	return map[string]any{"name": name, "source": source, "bindings": bindings, "valueWoVersion": version}, errs
}

func written(kind string, existing *resource, fields, body map[string]any) (string, []fieldError) {
	switch kind {
	case "secret":
		return secretValue(existing, fields, body)
	case "runner-pool":
		return poolAccess(existing, fields, body)
	case "egress":
		return egressToken(existing, fields, body)
	case "hook":
		return hookSecret(existing, fields, body)
	default:
		return "", nil
	}
}

func secretValue(existing *resource, fields, body map[string]any) (string, []fieldError) {
	value, given := body["valueWo"].(string)
	builtIn := objectOf(fields["source"])["kind"] == "built-in"

	switch {
	case !builtIn && given:
		return "", []fieldError{{"valueWo", "is kept only by Spotlight; a secret read from AWS Secrets Manager is set there"}}
	case !builtIn:
		fields["rotatedAt"] = rotatedAt(existing)
		return "", nil
	}

	needed := existing == nil ||
		objectOf(existing.fields["source"])["kind"] != "built-in" ||
		existing.fields["valueWoVersion"] != fields["valueWoVersion"]

	if needed && !given {
		return "", []fieldError{{"valueWo", "is the value Spotlight keeps; it is written once and never read back"}}
	}

	if !needed {
		fields["rotatedAt"] = rotatedAt(existing)
		return "", nil
	}

	fields["rotatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)

	return value, nil
}

func rotatedAt(existing *resource) any {
	if existing == nil {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}

	return existing.fields["rotatedAt"]
}

var (
	tokenTaken = map[string]string{
		"runner-pool": "is already another pool’s token; give each pool its own",
		"egress":      "belongs to another egress. Please give each egress its own",
	}
	tokenSpent = map[string]string{
		"runner-pool": "was given to a pool before, and a pool token is given once, ever: a token that was rotated away, or that a deleted or replaced pool used, never works again. Give a new one",
		"egress":      "was used before. Egress tokens work once only",
	}
)

const pullOnly = "is only for a pool that is pulled from; a pool that provisions is called instead, and has no token"

func runnerPool(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	key, _ := body["key"].(string)
	key = strings.TrimSpace(key)
	if why := wordsLike("ephemeral")(key); why != "" {
		errs = append(errs, fieldError{"key", why})
	}

	fields := map[string]any{
		"key":          key,
		"labels":       onceEach(texts(body["labels"]), "labels", wordsLike("vpc-internal"), &errs),
		"maxRuns":      nil,
		"mode":         "pull",
		"tokenVersion": nil,
		"provision":    nil,
	}

	if maxRuns, ok := body["maxRuns"].(float64); ok {
		if maxRuns != float64(int64(maxRuns)) || maxRuns < 1 || maxRuns > 10000 {
			errs = append(errs, fieldError{"maxRuns", "is a whole number from 1 to 10000"})
		}
		fields["maxRuns"] = maxRuns
	}

	given, ok := body["provision"].(map[string]any)
	if !ok {
		fields["tokenVersion"] = versionOf(body["tokenVersion"])
		return fields, errs
	}

	for _, field := range []string{"token", "tokenVersion"} {
		if body[field] != nil {
			errs = append(errs, fieldError{field, pullOnly})
		}
	}

	written, _ := given["url"].(string)
	called, err := url.Parse(written)
	if err != nil || !called.IsAbs() || called.Scheme != "https" || called.User != nil || called.Fragment != "" {
		errs = append(errs, fieldError{"provision.url", "is an absolute https URL, as in https://provision.example.com/runs"})
	}

	fields["mode"] = "push"
	fields["provision"] = map[string]any{"url": written, "secretVersion": versionOf(given["secretVersion"]), "egress": nil, "egressTraffic": "all"}
	named, _ := given["egress"].(string)
	if strings.TrimSpace(named) != "" {
		objectOf(fields["provision"])["egress"] = strings.TrimSpace(named)
	}
	switch traffic := given["egressTraffic"]; {
	case traffic == nil:
	case traffic != "all" && traffic != "bound":
		errs = append(errs, fieldError{"provision.egressTraffic", "is all or bound"})
	case strings.TrimSpace(named) == "":
		errs = append(errs, fieldError{"provision.egressTraffic", "is only for a pool that names an egress in provision.egress"})
	default:
		objectOf(fields["provision"])["egressTraffic"] = traffic
	}

	return fields, errs
}

func egress(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	key, _ := body["key"].(string)
	key = strings.TrimSpace(key)
	if why := wordsLike("shared")(key); why != "" {
		errs = append(errs, fieldError{"key", why})
	}

	written, _ := body["url"].(string)
	written = strings.TrimSpace(written)
	sent, err := url.Parse(written)
	switch {
	case err != nil || !sent.IsAbs() || sent.Host == "":
		errs = append(errs, fieldError{"url", "must be an absolute https URL, as in https://egress.example.com"})
	case sent.Scheme != "https" && sent.Scheme != "http":
		errs = append(errs, fieldError{"url", "must be an http or https URL"})
	case sent.User != nil:
		errs = append(errs, fieldError{"url", "must carry no credentials. Runners give their proxy credential"})
	case sent.RawQuery != "" || sent.Fragment != "" || len(written) > 2048:
		errs = append(errs, fieldError{"url", "must be at most 2048 characters, with no query and no fragment"})
	}

	return map[string]any{"key": key, "url": written, "tokenVersion": versionOf(body["tokenVersion"])}, errs
}

func egressToken(existing *resource, fields, body map[string]any) (string, []fieldError) {
	token, _ := body["token"].(string)

	switch {
	case token != "" && !runnerKey.MatchString(token):
		return "", []fieldError{{"token", "must be 32 to 255 URL-safe characters"}}
	case token == "" && existing != nil && existing.fields["tokenVersion"] != fields["tokenVersion"]:
		return "", []fieldError{{"token", "is required whenever tokenVersion changes"}}
	default:
		return token, nil
	}
}

func versionOf(value any) any {
	if version, ok := value.(float64); ok {
		return version
	}

	return nil
}

func poolAccess(existing *resource, fields, body map[string]any) (string, []fieldError) {
	if existing != nil && existing.fields["mode"] != fields["mode"] {
		return "", []fieldError{{"provision", fmt.Sprintf("The %s runner pool is %s for its whole life; replace it to be %s instead", textOf(fields["key"]), existing.fields["mode"], fields["mode"])}}
	}

	if fields["mode"] == "pull" {
		token, _ := body["token"].(string)
		switch {
		case token != "" && !runnerKey.MatchString(token):
			return "", []fieldError{{"token", "is 32 to 255 URL-safe characters, as a generated password would be"}}
		case token == "" && existing != nil && existing.fields["tokenVersion"] != fields["tokenVersion"]:
			return "", []fieldError{{"token", "is given whenever tokenVersion changes, since that is how it is rotated; Spotlight issues one only when a pool is created without it"}}
		default:
			return token, nil
		}
	}

	secret, given := objectOf(body["provision"])["secret"].(string)
	if given && !signingSecret(secret) {
		return "", []fieldError{{"provision.secret", "is whsec_ followed by 24 to 64 base64-encoded random bytes, as Standard Webhooks signing secrets are"}}
	}

	switch {
	case given:
		return secret, nil
	case existing == nil:
		return "", []fieldError{{"provision.secret", "is given when a pool that provisions is defined, to sign its calls with"}}
	case objectOf(existing.fields["provision"])["secretVersion"] != objectOf(fields["provision"])["secretVersion"]:
		return "", []fieldError{{"provision.secret", "is given again whenever provision.secretVersion changes, since that is how it is rotated"}}
	default:
		return "", nil
	}
}

func signingSecret(secret string) bool {
	encoded, ok := strings.CutPrefix(secret, "whsec_")
	decoded, err := base64.StdEncoding.DecodeString(encoded)

	return ok && err == nil && len(decoded) >= 24 && len(decoded) <= 64
}

func issuer(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	loopback := err == nil && parsed.Scheme == "http" && slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, parsed.Hostname())

	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || len(trimmed) > 255 {
		return "is the https URL the platform signs its tokens as, with no query or fragment, as in https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE"
	}

	return ""
}

func oneWord(raw, example string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 255 || strings.ContainsAny(trimmed, " \t\r\n") {
		return trimmed, "is one word of at most 255 characters, as in " + example
	}

	return trimmed, ""
}

func runnerTrust(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	key, _ := body["key"].(string)
	key = strings.TrimSpace(key)
	if why := wordsLike("production-cluster")(key); why != "" {
		errs = append(errs, fieldError{"key", why})
	}

	issued, _ := body["issuer"].(string)
	if why := issuer(issued); why != "" {
		errs = append(errs, fieldError{"issuer", why})
	}

	written, _ := body["subject"].(string)
	subject, why := oneWord(written, "system:serviceaccount:spotlight:runner")
	if why != "" {
		errs = append(errs, fieldError{"subject", why})
	}

	written, given := body["audience"].(string)
	if !given {
		written = "spotlight"
	}
	audience, why := oneWord(written, "spotlight")
	if why != "" {
		errs = append(errs, fieldError{"audience", why})
	}

	return map[string]any{
		"key":      key,
		"issuer":   strings.TrimSpace(issued),
		"subject":  subject,
		"audience": audience,
		"labels":   onceEach(texts(body["labels"]), "labels", wordsLike("vpc-internal"), &errs),
	}, errs
}

func workspaceRunners(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	return map[string]any{
		"trusts": onceEach(texts(body["trusts"]), "trusts", wordsLike("production-cluster"), &errs),
		"pools":  onceEach(texts(body["pools"]), "pools", wordsLike("ephemeral"), &errs),
	}, errs
}

func (s *Server) unansweredOf(declared *resource) []any {
	trusts := stringsOf(declared.fields["trusts"])
	pools := stringsOf(declared.fields["pools"])
	known, pooled := map[string]bool{}, map[string]bool{}
	warnings := []any{}

	for _, each := range s.stored {
		if s.workspaceOf(each) != declared.owner {
			continue
		}

		switch each.kind {
		case "runner-trust":
			known[textOf(each.fields["key"])] = true
		case "runner-pool":
			pooled[textOf(each.fields["key"])] = true
		}
	}

	for at, key := range trusts {
		if !known[key] {
			warnings = append(warnings, warned(fmt.Sprintf("trusts[%d]", at), fmt.Sprintf("names %s, but the workspace trusts no workload by that key", key)))
		}
	}

	for at, key := range pools {
		if !pooled[key] {
			warnings = append(warnings, warned(fmt.Sprintf("pools[%d]", at), fmt.Sprintf("names %s, but the workspace has no runner pool by that key", key)))
		}
	}

	return warnings
}

func warned(path, message string) map[string]any {
	return map[string]any{"path": path, "message": message, "severity": "warning"}
}

func stringsOf(value any) []string {
	out := []string{}
	for _, each := range texts(value) {
		out = append(out, textOf(each))
	}

	return out
}

func harnessProfile(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	key, _ := body["key"].(string)
	key = strings.TrimSpace(key)
	if !words(key) {
		errs = append(errs, fieldError{"key", fmt.Sprintf("is lower case words joined by dashes, at most %d characters, as in careful", longestWords)})
	}

	given, _ := body["permissions"].(map[string]any)
	permissions := map[string]any{}
	stated := map[string]string{}
	for _, decision := range []string{"allow", "ask", "deny"} {
		listed := []any{}
		for at, each := range texts(given[decision]) {
			written, ok := permissionRule(textOf(each))
			path := fmt.Sprintf("permissions.%s[%d]", decision, at)
			if !ok {
				errs = append(errs, fieldError{path, "names what the harness may do as read, edit, execute, alone or with a pattern, as in execute(npm test *) or read(.env)"})
			}
			switch earlier, seen := stated[written]; {
			case seen && earlier == decision:
				errs = append(errs, fieldError{path, fmt.Sprintf("names %s twice under %s", written, decision)})
			case seen:
				errs = append(errs, fieldError{path, fmt.Sprintf("names %s under both %s and %s; a rule says one thing", written, earlier, decision)})
			default:
				stated[written] = decision
			}
			listed = append(listed, written)
		}
		permissions[decision] = listed
	}

	listed, _ := body["mcpServers"].([]any)
	servers := make([]any, 0, len(listed))
	names := map[string]bool{}
	for index, each := range listed {
		server := maps.Clone(objectOf(each))
		at := fmt.Sprintf("mcpServers[%d]", index)
		name, _ := server["name"].(string)
		name = strings.TrimSpace(name)
		if !words(name) {
			errs = append(errs, fieldError{at + ".name", "names an MCP server in lower case words joined by dashes, as in linear"})
		}
		if names[name] {
			errs = append(errs, fieldError{at + ".name", fmt.Sprintf("names the MCP server %s twice", name)})
		}
		names[name] = true
		command, _ := server["command"].(string)
		if strings.TrimSpace(command) == "" {
			errs = append(errs, fieldError{at + ".command", "starts an MCP server with a command, as in npx"})
		}
		orDefault(server, "env", map[string]any{})
		for variable := range objectOf(server["env"]) {
			if !envName.MatchString(variable) {
				errs = append(errs, fieldError{keyed(at+".env", variable), "names an environment variable in letters, digits and underscores"})
			}
		}
		server["name"], server["command"], server["args"] = name, strings.TrimSpace(command), texts(server["args"])
		servers = append(servers, server)
	}

	native, ok := body["native"].(map[string]any)
	if !ok {
		native = map[string]any{}
	}

	errs = append(errs, nativeProblems(native)...)

	return map[string]any{"key": key, "permissions": permissions, "mcpServers": servers, "native": native}, errs
}

func permissionRule(raw string) (string, bool) {
	matched := rule.FindStringSubmatch(strings.TrimSpace(raw))
	if matched == nil || !slices.Contains([]string{"read", "edit", "execute"}, matched[1]) {
		return raw, false
	}

	if !strings.HasSuffix(matched[0], ")") {
		return matched[1], true
	}

	pattern := strings.TrimSpace(matched[2])

	return matched[1] + "(" + pattern + ")", pattern != ""
}

func nativeProblems(native map[string]any) []fieldError {
	var errs []fieldError

	for _, harness := range slices.Sorted(maps.Keys(native)) {
		at := keyed("native", harness)

		switch format, known := harnesses[harness]; {
		case !known:
			errs = append(errs, fieldError{at, "names no harness Spotlight runs; it runs claude-code, cursor, codex"})
		case format == "json":
			if _, ok := native[harness].(map[string]any); !ok {
				errs = append(errs, fieldError{at, "is the harness's own settings, as a JSON object"})
			}
		default:
			if _, ok := native[harness].(string); !ok {
				errs = append(errs, fieldError{at, "is the harness's own config.toml, as text"})
			}
		}
	}

	return errs
}

var tokenScopes = []string{"read", "plan", "configure", "secrets", "runners", "trusts"}

func tokenTrust(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	key, _ := body["key"].(string)
	key = strings.TrimSpace(key)
	if why := wordsLike("github-main")(key); why != "" {
		errs = append(errs, fieldError{"key", why})
	}

	issued, _ := body["issuer"].(string)
	if why := issuer(issued); why != "" {
		errs = append(errs, fieldError{"issuer", why})
	}

	written, _ := body["subject"].(string)
	subject, why := oneWord(written, "repo:acme/platform:ref:refs/heads/main")
	if why != "" {
		errs = append(errs, fieldError{"subject", why})
	}
	if owner, _, wild := strings.Cut(subject, "*"); wild && !strings.Contains(owner, "/") {
		errs = append(errs, fieldError{"subject", "names its owner before any *, as in repo:acme/platform:*"})
	}

	written, given := body["audience"].(string)
	if !given {
		written = "spotlight"
	}
	audience, why := oneWord(written, "spotlight")
	if why != "" {
		errs = append(errs, fieldError{"audience", why})
	}

	claims := map[string]any{}
	for name, pattern := range objectOf(body["claims"]) {
		if text, ok := pattern.(string); ok && text != "" {
			claims[name] = text
			continue
		}
		errs = append(errs, fieldError{"claims." + name, "is a pattern the claim must match, as in acme/platform"})
	}

	scopes := onceEach(texts(body["scopes"]), "scopes", func(scope string) string {
		if slices.Contains(tokenScopes, scope) {
			return ""
		}
		return "is one of " + strings.Join(tokenScopes, ", ")
	}, &errs)
	if len(scopes) == 0 {
		errs = append(errs, fieldError{"scopes", "names at least one scope, as in plan"})
	}

	lifetime := 15.0
	if minutes, ok := body["lifetimeMinutes"].(float64); ok {
		lifetime = minutes
	}
	if lifetime < 1 || lifetime > 60 || lifetime != float64(int(lifetime)) {
		errs = append(errs, fieldError{"lifetimeMinutes", "is a whole number of minutes from 1 to 60"})
	}

	return map[string]any{
		"key":             key,
		"issuer":          strings.TrimSpace(issued),
		"audience":        audience,
		"subject":         subject,
		"claims":          claims,
		"scopes":          scopes,
		"lifetimeMinutes": lifetime,
	}, errs
}
