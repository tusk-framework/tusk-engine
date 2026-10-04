package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// HTTPReadinessProbe uses RoadRunner's status plugin readiness endpoint.
type HTTPReadinessProbe struct {
	URL    string
	Client *http.Client
}

// NewHTTPReadinessProbe creates a probe for a loopback status address.
func NewHTTPReadinessProbe(address string) HTTPReadinessProbe {
	return HTTPReadinessProbe{URL: "http://" + strings.TrimSpace(address) + "/ready?plugin=http"}
}

func (p HTTPReadinessProbe) Check(ctx context.Context) error {
	if strings.TrimSpace(p.URL) == "" {
		return fmt.Errorf("%w: empty readiness URL", ErrProbeFailed)
	}
	parsed, err := url.Parse(p.URL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return fmt.Errorf("%w: invalid readiness URL", ErrProbeFailed)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return fmt.Errorf("%w: create readiness request", ErrProbeFailed)
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: readiness transport", ErrProbeFailed)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	if response.StatusCode == http.StatusServiceUnavailable {
		return ErrNotReady
	}
	return fmt.Errorf("%w: unexpected readiness status", ErrProbeFailed)
}
