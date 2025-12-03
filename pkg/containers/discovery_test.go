package containers

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestMatchesTraefikRule(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		hostname string
		expected bool
	}{
		{
			name: "exact Host() match",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`webserver.itxnet.local`)",
			},
			hostname: "webserver.itxnet.local",
			expected: true,
		},
		{
			name: "Host() no match",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`webserver.itxnet.local`)",
			},
			hostname: "other.itxnet.local",
			expected: false,
		},
		{
			name: "multiple Host() match first",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`web.example.com`, `webserver.itxnet.local`)",
			},
			hostname: "web.example.com",
			expected: true,
		},
		{
			name: "multiple Host() match second",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`web.example.com`, `webserver.itxnet.local`)",
			},
			hostname: "webserver.itxnet.local",
			expected: true,
		},
		{
			name: "HostRegexp() simple pattern match",
			labels: map[string]string{
				"traefik.http.routers.lazyload.rule": "HostRegexp(`^[\\w\\d-]+\\.itxnet\\.local$`)",
			},
			hostname: "webserver.itxnet.local",
			expected: true,
		},
		{
			name: "HostRegexp() pattern match with subdomain",
			labels: map[string]string{
				"traefik.http.routers.lazyload.rule": "HostRegexp(`^[\\w\\d-]+\\.itxnet\\.local$`)",
			},
			hostname: "test-123.itxnet.local",
			expected: true,
		},
		{
			name: "HostRegexp() no match - wrong domain",
			labels: map[string]string{
				"traefik.http.routers.lazyload.rule": "HostRegexp(`^[\\w\\d-]+\\.itxnet\\.local$`)",
			},
			hostname: "webserver.example.com",
			expected: false,
		},
		{
			name: "HostRegexp() no match - missing subdomain",
			labels: map[string]string{
				"traefik.http.routers.lazyload.rule": "HostRegexp(`^[\\w\\d-]+\\.itxnet\\.local$`)",
			},
			hostname: "itxnet.local",
			expected: false,
		},
		{
			name: "HostRegexp() with special characters",
			labels: map[string]string{
				"traefik.http.routers.api.rule": "HostRegexp(`^api-[a-z0-9]+\\.example\\.com$`)",
			},
			hostname: "api-test123.example.com",
			expected: true,
		},
		{
			name: "Host() and HostRegexp() combined - Host matches",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`exact.example.com`) || HostRegexp(`^[\\w\\d-]+\\.itxnet\\.local$`)",
			},
			hostname: "exact.example.com",
			expected: true,
		},
		{
			name: "Host() and HostRegexp() combined - HostRegexp matches",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`exact.example.com`) || HostRegexp(`^[\\w\\d-]+\\.itxnet\\.local$`)",
			},
			hostname: "test.itxnet.local",
			expected: true,
		},
		{
			name: "Host() with spaces",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`web.example.com` , `test.example.com`)",
			},
			hostname: "test.example.com",
			expected: true,
		},
		{
			name: "non-router label ignored",
			labels: map[string]string{
				"traefik.http.services.web.loadbalancer.server.port": "8080",
			},
			hostname: "webserver.itxnet.local",
			expected: false,
		},
		{
			name: "router label without .rule suffix ignored",
			labels: map[string]string{
				"traefik.http.routers.web.entrypoints": "http",
			},
			hostname: "webserver.itxnet.local",
			expected: false,
		},
		{
			name: "substring in hostname should not match",
			labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`server.local`)",
			},
			hostname: "webserver.local",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := &Wrapper{
				Summary: container.Summary{
					Labels: tt.labels,
				},
			}
			result := matchesTraefikRule(wrapper, tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesHostMatcher(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		hostname string
		expected bool
	}{
		{
			name:     "single host match",
			rule:     "Host(`example.com`)",
			hostname: "example.com",
			expected: true,
		},
		{
			name:     "single host no match",
			rule:     "Host(`example.com`)",
			hostname: "other.com",
			expected: false,
		},
		{
			name:     "multiple hosts first match",
			rule:     "Host(`a.com`, `b.com`, `c.com`)",
			hostname: "a.com",
			expected: true,
		},
		{
			name:     "multiple hosts middle match",
			rule:     "Host(`a.com`, `b.com`, `c.com`)",
			hostname: "b.com",
			expected: true,
		},
		{
			name:     "multiple hosts last match",
			rule:     "Host(`a.com`, `b.com`, `c.com`)",
			hostname: "c.com",
			expected: true,
		},
		{
			name:     "multiple hosts no match",
			rule:     "Host(`a.com`, `b.com`, `c.com`)",
			hostname: "d.com",
			expected: false,
		},
		{
			name:     "complex rule with Host",
			rule:     "Host(`example.com`) && PathPrefix(`/api`)",
			hostname: "example.com",
			expected: true,
		},
		{
			name:     "no Host matcher",
			rule:     "PathPrefix(`/api`)",
			hostname: "example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesHostMatcher(tt.rule, tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesHostRegexpMatcher(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		hostname string
		expected bool
	}{
		{
			name:     "simple regex match",
			rule:     "HostRegexp(`^[a-z]+\\.example\\.com$`)",
			hostname: "test.example.com",
			expected: true,
		},
		{
			name:     "simple regex no match - digits",
			rule:     "HostRegexp(`^[a-z]+\\.example\\.com$`)",
			hostname: "test123.example.com",
			expected: false,
		},
		{
			name:     "regex with word characters",
			rule:     "HostRegexp(`^[\\w\\d-]+\\.local$`)",
			hostname: "test-server_123.local",
			expected: true,
		},
		{
			name:     "regex anchors enforced",
			rule:     "HostRegexp(`^subdomain\\.example\\.com$`)",
			hostname: "test.subdomain.example.com",
			expected: false,
		},
		{
			name:     "complex regex pattern with character class",
			rule:     `HostRegexp(` + "`" + `^[a-z]+-[0-9]+[.]example[.]com$` + "`" + `)`,
			hostname: "api-123.example.com",
			expected: true,
		},
		{
			name:     "complex regex pattern no match",
			rule:     `HostRegexp(` + "`" + `^[a-z]+-[0-9]+[.]example[.]com$` + "`" + `)`,
			hostname: "api123.example.com",
			expected: false,
		},
		{
			name:     "multiple HostRegexp patterns first match",
			rule:     "HostRegexp(`^test\\..*$`, `^prod\\..*$`)",
			hostname: "test.example.com",
			expected: true,
		},
		{
			name:     "multiple HostRegexp patterns second match",
			rule:     "HostRegexp(`^test\\..*$`, `^prod\\..*$`)",
			hostname: "prod.example.com",
			expected: true,
		},
		{
			name:     "invalid regex should not match",
			rule:     "HostRegexp(`^[invalid(regex$`)",
			hostname: "anything.com",
			expected: false,
		},
		{
			name:     "no HostRegexp matcher",
			rule:     "Host(`example.com`)",
			hostname: "example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesHostRegexpMatcher(tt.rule, tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractBacktickValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single value",
			input:    "`example.com`",
			expected: []string{"example.com"},
		},
		{
			name:     "multiple values",
			input:    "`a.com`, `b.com`, `c.com`",
			expected: []string{"a.com", "b.com", "c.com"},
		},
		{
			name:     "values with spaces",
			input:    "`a.com` , `b.com`",
			expected: []string{"a.com", "b.com"},
		},
		{
			name:     "regex pattern",
			input:    "`^[\\w\\d-]+\\.local$`",
			expected: []string{`^[\w\d-]+\.local$`},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "no backticks",
			input:    "example.com",
			expected: []string{},
		},
		{
			name:     "mixed content",
			input:    "Host(`example.com`) || Path(`/api`)",
			expected: []string{"example.com", "/api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBacktickValues(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
