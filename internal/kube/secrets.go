package kube

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Secret contains only the fields the list and key inspector need.
type Secret struct {
	Namespace, Name, UID string
	Type                 string
	Helm                 bool
	Created              time.Time
	Keys                 []SecretKey
}

type SecretKey struct {
	Name  string
	Bytes int
}

// Emit only the expected fields from kubectl. In particular, metadata
// annotations can contain a previously applied manifest with credentials.
// Encode the custom type separately, since it can contain arbitrary text.
const secretListTemplate = `{{range .items}}{
"name":{{printf "%q" .metadata.name}},
"namespace":{{printf "%q" .metadata.namespace}},
"uid":{{printf "%q" .metadata.uid}},
"created":{{printf "%q" .metadata.creationTimestamp}},
"type":{{printf "%q" (printf "%x" .type)}},
"helm":{{if and .metadata.labels (eq (index .metadata.labels "app.kubernetes.io/managed-by") "Helm")}}true{{else}}false{{end}},
"keys":[{{$sep := ""}}{{range $key, $value := .data}}{{$sep}}{"name":{{printf "%q" $key}},"bytes":{{len (base64decode $value)}}}{{$sep = ","}}{{end}}]}
{{end}}`

func readSecrets(ctx context.Context, r run.Runner, env sysenv.Env, t Target, scopes [][]string) ([]Secret, error) {
	secrets := []Secret{}
	if t.API != nil && t.Kubectl == "" {
		for _, scope := range scopes {
			items, err := readSecretsAPI(ctx, env, t, scope)
			if err != nil {
				return nil, err
			}
			secrets = append(secrets, items...)
		}
		slices.SortFunc(secrets, func(a, b Secret) int {
			return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
		})
		return secrets, nil
	}
	for _, scope := range scopes {
		data, err := secretOutput(ctx, r, env, t, secretListTemplate, append([]string{"secrets"}, scope...)...)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		for {
			var secret Secret
			err := decoder.Decode(&secret)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				clear(data)
				return nil, errors.New("invalid kubectl Secret metadata")
			}
			kind, err := hex.DecodeString(secret.Type)
			if err != nil {
				clear(data)
				return nil, errors.New("invalid kubectl Secret type")
			}
			secret.Type = string(kind)
			secret.Helm = secret.Helm || secret.Type == "helm.sh/release.v1"
			secrets = append(secrets, secret)
		}
		clear(data)
	}
	slices.SortFunc(secrets, func(a, b Secret) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})
	return secrets, nil
}

type apiSecretObject struct {
	meta
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
}

func clearSecretRaw(data map[string]json.RawMessage) {
	for k, raw := range data {
		clear(raw)
		delete(data, k)
	}
}

func decodeBase64JSON(raw json.RawMessage) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
		return nil, errors.New("invalid Secret value")
	}
	encoded := trimmed[1 : len(trimmed)-1]
	value := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	size, err := base64.StdEncoding.Decode(value, encoded)
	if err != nil {
		clear(value)
		return nil, errors.New("invalid Secret value")
	}
	return value[:size], nil
}

func readSecretsAPI(ctx context.Context, env sysenv.Env, t Target, scope []string) ([]Secret, error) {
	var list struct {
		Items []apiSecretObject `json:"items"`
	}
	if err := t.API.get(ctx, env, t, append([]string{"secrets"}, scope...), &list); err != nil {
		if errors.Is(err, errUseKubectl) {
			return nil, &Error{Access: detect.KubeAccess{Detail: "kubectl is required to read secrets"}}
		}
		return nil, err
	}
	secrets := make([]Secret, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		keys := make([]SecretKey, 0, len(item.Data))
		for key, raw := range item.Data {
			decoded, err := decodeBase64JSON(raw)
			size := len(decoded)
			clear(decoded)
			clear(raw)
			if err != nil {
				clearSecretRaw(item.Data)
				return nil, err
			}
			keys = append(keys, SecretKey{Name: key, Bytes: size})
		}
		clearSecretRaw(item.Data)
		slices.SortFunc(keys, func(a, b SecretKey) int { return cmp.Compare(a.Name, b.Name) })
		m := item.Metadata
		helm := m.Labels["app.kubernetes.io/managed-by"] == "Helm" || item.Type == "helm.sh/release.v1"
		secrets = append(secrets, Secret{
			Namespace: m.Namespace, Name: m.Name, UID: m.UID,
			Type: item.Type, Helm: helm, Created: m.CreationTimestamp, Keys: keys,
		})
	}
	return secrets, nil
}

// ReadSecretValue returns the requested key only if the Secret still has
// the selected UID. The caller owns the decoded bytes and can clear them.
func ReadSecretValue(ctx context.Context, r run.Runner, env sysenv.Env, t Target, secret Secret, key string) ([]byte, error) {
	if t.API != nil && t.Kubectl == "" {
		var item apiSecretObject
		err := t.API.get(ctx, env, t, []string{"secrets/" + secret.Name, "--namespace=" + secret.Namespace}, &item)
		defer clearSecretRaw(item.Data)
		if err != nil {
			if errors.Is(err, errUseKubectl) {
				return nil, &Error{Access: detect.KubeAccess{Detail: "kubectl is required to read secrets"}}
			}
			return nil, err
		}
		if item.Metadata.UID != secret.UID {
			return nil, errors.New("the Secret or key changed")
		}
		raw, ok := item.Data[key]
		if !ok {
			return nil, errors.New("the Secret or key changed")
		}
		return decodeBase64JSON(raw)
	}
	template := fmt.Sprintf(`{{if eq .metadata.uid %q}}{{range $key, $value := .data}}{{if eq $key %q}}={{$value}}{{end}}{{end}}{{end}}`, secret.UID, key)
	data, err := secretOutput(ctx, r, env, t, template, "secret/"+secret.Name, "--namespace="+secret.Namespace)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	if len(data) == 0 || data[0] != '=' {
		return nil, errors.New("the Secret or key changed")
	}
	encoded := data[1:]
	value := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	size, err := base64.StdEncoding.Decode(value, encoded)
	if err != nil {
		clear(value)
		return nil, errors.New("invalid kubectl Secret value")
	}
	return value[:size], nil
}

func secretOutput(ctx context.Context, r run.Runner, env sysenv.Env, t Target, template string, args ...string) ([]byte, error) {
	if t.Kubectl == "" {
		return nil, &Error{Access: detect.KubeAccess{Detail: "kubectl is required to read secrets"}}
	}
	argv := append([]string{t.Kubectl, "--context=" + t.Context, "get"}, args...)
	argv = append(argv, "--output=go-template="+template, "--request-timeout=15s")
	out, err := r.Run(ctx, run.Cmd{Args: argv, Env: env, Timeout: timeout, MaxOutput: 64 << 20})
	defer clear(out.Stderr)
	if err != nil || out.Truncated || out.ExitCode != 0 {
		clear(out.Stdout)
		// kubectl and credential plugins may include input in diagnostics.
		// Keep the classified access state only.
		return nil, &Error{Access: detect.KubeAccess{State: detect.KubeFailure(string(out.Stderr)).State}}
	}
	return out.Stdout, nil
}
