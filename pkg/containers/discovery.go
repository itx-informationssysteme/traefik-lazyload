package containers

import (
	"context"
	"regexp"
	"strings"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

type Discovery struct {
	client Host
}

func NewDiscovery(client Host) *Discovery {
	return &Discovery{client}
}

// Return all containers that qualify to be load-managed (eg. have the tag)
func (s *Discovery) QualifyingContainers(ctx context.Context) ([]Wrapper, error) {
	return s.FindAllLazyload(ctx, true)
}

func (s *Discovery) ProviderContainers(ctx context.Context) ([]Wrapper, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.SubLabel("provides"))

	return wrapListResult(s.client.ContainerList(ctx, container.ListOptions{
		Filters: filters,
		All:     true,
	}))
}

func (s *Discovery) FindAllLazyload(ctx context.Context, includeStopped bool) ([]Wrapper, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.Model.LabelPrefix)

	return wrapListResult(s.client.ContainerList(ctx, container.ListOptions{
		All:     includeStopped,
		Filters: filters,
	}))
}

func (s *Discovery) FindContainerByHostname(ctx context.Context, hostname string) (*Wrapper, error) {
	containers, err := s.FindAllLazyload(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if hostStr, ok := c.Config("hosts"); ok {
			hosts := strings.Split(hostStr, ",")
			if strSliceContains(hosts, hostname) {
				return &c, nil
			}
		} else if matchesTraefikRule(&c, hostname) {
			// If not defined explicitly, infer from traefik route
			return &c, nil
		}
	}

	return nil, ErrNotFound
}

// matchesTraefikRule checks if hostname matches any Traefik router rule
func matchesTraefikRule(c *Wrapper, hostname string) bool {
	for k, v := range c.Labels {
		if strings.Contains(k, "traefik.http.routers.") && strings.HasSuffix(k, ".rule") {
			if matchesTraefikRuleValue(v, hostname) {
				return true
			}
		}
	}
	return false
}

// matchesTraefikRuleValue parses Traefik rule syntax and checks if hostname matches
func matchesTraefikRuleValue(rule string, hostname string) bool {
	// Check Host() matchers
	if matchesHostMatcher(rule, hostname) {
		return true
	}

	// Check HostRegexp() matchers
	if matchesHostRegexpMatcher(rule, hostname) {
		return true
	}

	return false
}

// matchesHostMatcher checks if hostname matches Host() directive
func matchesHostMatcher(rule string, hostname string) bool {
	hostPattern := regexp.MustCompile(`Host\((.*?)\)`)
	matches := hostPattern.FindAllStringSubmatch(rule, -1)
	for _, match := range matches {
		if len(match) > 1 {
			hosts := extractBacktickValues(match[1])
			for _, host := range hosts {
				if host == hostname {
					return true
				}
			}
		}
	}
	return false
}

// matchesHostRegexpMatcher checks if hostname matches HostRegexp() directive
func matchesHostRegexpMatcher(rule string, hostname string) bool {
	hostRegexpPattern := regexp.MustCompile(`HostRegexp\((.*?)\)`)
	matches := hostRegexpPattern.FindAllStringSubmatch(rule, -1)
	for _, match := range matches {
		if len(match) > 1 {
			patterns := extractBacktickValues(match[1])
			for _, pattern := range patterns {
				if re, err := regexp.Compile(pattern); err == nil {
					if re.MatchString(hostname) {
						return true
					}
				}
			}
		}
	}
	return false
}

// extractBacktickValues extracts values from backtick-quoted strings
// e.g., "`a.com`, `b.com`" -> ["a.com", "b.com"]
func extractBacktickValues(s string) []string {
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatch(s, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			result = append(result, match[1])
		}
	}
	return result
}

func (s *Discovery) FindDepProvider(ctx context.Context, name string) ([]Wrapper, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.SubLabel("provides")+"="+name)
	return wrapListResult(s.client.ContainerList(ctx, container.ListOptions{
		Filters: filters,
		All:     true,
	}))
}
