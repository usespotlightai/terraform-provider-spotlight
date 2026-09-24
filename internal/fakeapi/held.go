package fakeapi

import (
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

func runnerName(text string) string {
	switch {
	case text == "":
		return "must not be empty"
	case len(text) > 60:
		return "must be at most 60 characters"
	default:
		return ""
	}
}

func onceEach(held []any, at string, wrong func(string) string, errs *[]fieldError) []any {
	seen := map[string]bool{}
	out := make([]any, 0, len(held))

	for index, each := range held {
		text := strings.TrimSpace(each.(string))
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
		header, ok := binding["header"].(string)
		if !ok {
			header = "authorization"
		}
		prefix, _ := binding["prefix"].(string)

		header = strings.ToLower(strings.TrimSpace(header))

		if !hostShape.MatchString(host) || len(host) > 253 {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].host", at), "is the one host the secret may be sent to, as in api.linear.app, with no scheme, port or path"})
		}
		if !headerName.MatchString(header) || slices.Contains(neverWorn, header) {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].header", at), "is the request header the secret is worn in, as in authorization or x-api-key"})
		}
		if !prefixText.MatchString(prefix) {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].prefix", at), `is up to 64 printable characters written before the value, as in "Bearer "`})
		}
		if hosts[host] {
			errs = append(errs, fieldError{fmt.Sprintf("bindings[%d].host", at), fmt.Sprintf("binds %s twice; a secret is worn one way toward each host", host)})
		}
		hosts[host] = true

		bindings = append(bindings, map[string]any{"host": host, "header": header, "prefix": prefix})
	}

	var version any
	if held, ok := body["valueWoVersion"].(float64); ok {
		version = held
	}

	return map[string]any{"name": name, "source": source, "bindings": bindings, "valueWoVersion": version}, errs
}

func written(kind string, held *resource, fields, body map[string]any) (string, []fieldError) {
	switch kind {
	case "secret":
		return secretValue(held, fields, body)
	case "runner":
		token, _ := body["token"].(string)
		if token != "" && !runnerKey.MatchString(token) {
			return "", []fieldError{{"token", "is 32 to 255 URL-safe characters, as a generated password would be"}}
		}
		if held != nil {
			fields["trust"] = held.fields["trust"]
		}
		return token, nil
	default:
		return "", nil
	}
}

func secretValue(held *resource, fields, body map[string]any) (string, []fieldError) {
	value, given := body["valueWo"].(string)
	builtIn := fields["source"].(map[string]any)["kind"] == "built-in"

	switch {
	case !builtIn && given:
		return "", []fieldError{{"valueWo", "is kept only by Spotlight; a secret read from AWS Secrets Manager is set there"}}
	case !builtIn:
		fields["rotatedAt"] = rotatedAt(held)
		return "", nil
	}

	needed := held == nil ||
		held.fields["source"].(map[string]any)["kind"] != "built-in" ||
		held.fields["valueWoVersion"] != fields["valueWoVersion"]

	if needed && !given {
		return "", []fieldError{{"valueWo", "is the value Spotlight keeps; it is written once and never read back"}}
	}

	if !needed {
		fields["rotatedAt"] = rotatedAt(held)
		return "", nil
	}

	fields["rotatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)

	return value, nil
}

func rotatedAt(held *resource) any {
	if held == nil {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}

	return held.fields["rotatedAt"]
}

func runner(body map[string]any) (map[string]any, []fieldError) {
	var errs []fieldError

	name, _ := body["name"].(string)
	name = strings.TrimSpace(name)
	if why := runnerName(name); why != "" {
		errs = append(errs, fieldError{"name", why})
	}

	labels := onceEach(texts(body["labels"]), "labels", wordsLike("vpc-internal"), &errs)

	return map[string]any{"name": name, "labels": labels, "trust": nil}, errs
}

func issuer(raw string) string {
	held := strings.TrimSpace(raw)
	parsed, err := url.Parse(held)
	loopback := err == nil && parsed.Scheme == "http" && slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, parsed.Hostname())

	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || len(held) > 255 {
		return "is the https URL the platform signs its tokens as, with no query or fragment, as in https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE"
	}

	return ""
}

func oneWord(raw, example string) (string, string) {
	held := strings.TrimSpace(raw)
	if held == "" || len(held) > 255 || strings.ContainsAny(held, " \t\r\n") {
		return held, "is one word of at most 255 characters, as in " + example
	}

	return held, ""
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
		"runners": onceEach(texts(body["runners"]), "runners", runnerName, &errs),
		"trusts":  onceEach(texts(body["trusts"]), "trusts", wordsLike("production-cluster"), &errs),
	}, errs
}

func (s *Server) driftOf(declared *resource) ([]any, []any) {
	runners := stringsOf(declared.fields["runners"])
	trusts := stringsOf(declared.fields["trusts"])
	enrolled, known := map[string]bool{}, map[string]bool{}
	drift, warnings := []any{}, []any{}

	for _, each := range s.held {
		if s.workspaceOf(each) != declared.owner {
			continue
		}

		switch each.kind {
		case "runner-trust":
			known[each.fields["key"].(string)] = true
		case "runner":
			name := each.fields["name"].(string)
			enrolled[name] = true
			trust, _ := each.fields["trust"].(string)
			if !slices.Contains(runners, name) && (trust == "" || !slices.Contains(trusts, trust)) {
				drift = append(drift, name)
			}
		}
	}

	for at, name := range runners {
		if !enrolled[name] {
			warnings = append(warnings, warned(fmt.Sprintf("runners[%d]", at), fmt.Sprintf("names %s, but no runner by that name is enrolled", name)))
		}
	}

	for at, key := range trusts {
		if !known[key] {
			warnings = append(warnings, warned(fmt.Sprintf("trusts[%d]", at), fmt.Sprintf("names %s, but the workspace trusts no workload by that key", key)))
		}
	}

	slices.SortFunc(drift, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
	for _, name := range drift {
		warnings = append(warnings, warned("runners", fmt.Sprintf("leaves out %s, which is enrolled all the same; add it here or delete it", name)))
	}

	return drift, warnings
}

func warned(path, message string) map[string]any {
	return map[string]any{"path": path, "message": message, "severity": "warning"}
}

func stringsOf(held any) []string {
	out := []string{}
	for _, each := range texts(held) {
		out = append(out, each.(string))
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
			written, ok := permissionRule(each.(string))
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
		server := maps.Clone(each.(map[string]any))
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
		for variable := range server["env"].(map[string]any) {
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
