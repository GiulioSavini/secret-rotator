package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	// ComposeServiceLabel is set by Docker Compose on every container it creates.
	ComposeServiceLabel = "com.docker.compose.service"
	// ComposeProjectLabel identifies the Compose project a container belongs to.
	ComposeProjectLabel = "com.docker.compose.project"
	// ComposeNumberLabel is the replica index within a scaled Compose service.
	ComposeNumberLabel = "com.docker.compose.container-number"
)

// ResolveContainerNames maps the names written in rotator.yml onto the actual
// container names Docker knows about.
//
// Compose derives container names as "<project>-<service>-<n>", so a config
// that lists the service name ("db") does not match any container unless the
// user pinned container_name. This resolves each requested entry by, in order:
//
//  1. an exact container name match;
//  2. the com.docker.compose.service label, expanding scaled services into all
//     their replicas in replica order.
//
// Resolution is best-effort by design: when the daemon cannot be queried, or
// reports no containers at all, the requested names are returned unchanged so
// that behaviour matches a plain `docker restart`. A name that matches nothing
// while other containers do exist is a real misconfiguration and returns an
// error naming the candidates.
func ResolveContainerNames(ctx context.Context, mgr Manager, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	all, err := mgr.ListContainers(ctx, ContainerFilter{})
	if err != nil || len(all) == 0 {
		// No usable inventory: fall through to the raw names.
		return append([]string(nil), requested...), nil
	}

	byName := make(map[string]Container, len(all))
	byService := make(map[string][]Container)
	for _, c := range all {
		if c.Name != "" {
			byName[c.Name] = c
		}
		if svc := c.Labels[ComposeServiceLabel]; svc != "" {
			byService[svc] = append(byService[svc], c)
		}
	}

	resolved := make([]string, 0, len(requested))
	for _, want := range requested {
		if c, ok := byName[want]; ok {
			resolved = append(resolved, c.Name)
			continue
		}

		matches := byService[want]
		switch {
		case len(matches) == 0:
			return nil, fmt.Errorf(
				"container %q not found: no container has that name and no Compose service matches it.\nAvailable: %s",
				want, describeInventory(all))

		case len(matches) == 1:
			resolved = append(resolved, matches[0].Name)

		default:
			projects := distinctProjects(matches)
			if len(projects) > 1 {
				return nil, fmt.Errorf(
					"container %q is ambiguous: Compose service %q exists in projects %s; use the exact container name instead",
					want, want, strings.Join(projects, ", "))
			}
			// A single scaled service: restart every replica, lowest first.
			sortByReplica(matches)
			for _, c := range matches {
				resolved = append(resolved, c.Name)
			}
		}
	}

	return resolved, nil
}

// distinctProjects returns the sorted set of Compose projects across matches.
func distinctProjects(matches []Container) []string {
	seen := make(map[string]bool, len(matches))
	var projects []string
	for _, c := range matches {
		p := c.Labels[ComposeProjectLabel]
		if p == "" {
			p = "(none)"
		}
		if !seen[p] {
			seen[p] = true
			projects = append(projects, p)
		}
	}
	sort.Strings(projects)
	return projects
}

// sortByReplica orders scaled replicas by their Compose container number,
// falling back to the container name when the label is absent.
func sortByReplica(matches []Container) {
	sort.Slice(matches, func(i, j int) bool {
		ni := matches[i].Labels[ComposeNumberLabel]
		nj := matches[j].Labels[ComposeNumberLabel]
		if ni != nj && ni != "" && nj != "" {
			if len(ni) != len(nj) {
				return len(ni) < len(nj)
			}
			return ni < nj
		}
		return matches[i].Name < matches[j].Name
	})
}

// describeInventory renders the container names and Compose services that are
// available, so a "not found" error tells the user what they could have meant.
func describeInventory(all []Container) string {
	names := make([]string, 0, len(all))
	serviceSet := make(map[string]bool)
	for _, c := range all {
		if c.Name != "" {
			names = append(names, c.Name)
		}
		if svc := c.Labels[ComposeServiceLabel]; svc != "" {
			serviceSet[svc] = true
		}
	}
	sort.Strings(names)

	out := "containers [" + strings.Join(names, ", ") + "]"
	if len(serviceSet) > 0 {
		services := make([]string, 0, len(serviceSet))
		for s := range serviceSet {
			services = append(services, s)
		}
		sort.Strings(services)
		out += ", Compose services [" + strings.Join(services, ", ") + "]"
	}
	return out
}
