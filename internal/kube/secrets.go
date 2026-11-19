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

// ReadSecretValue returns the requested key only if the Secret still has
// the selected UID. The caller owns the decoded bytes and can clear them.
func ReadSecretValue(ctx context.Context, r run.Runner, env sysenv.Env, t Target, secret Secret, key string) ([]byte, error) {
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
