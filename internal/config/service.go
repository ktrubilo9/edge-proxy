package config

import (
	"errors"
	"fmt"
	"sync"
)

type Service struct {
	mu       sync.RWMutex
	path     string
	current  *FullConfig
	snapshot *Snapshot
}

func NewService(path string) (*Service, error) {
	if path == "" {
		return nil, errors.New("config path is required")
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	return &Service{
		path:     path,
		current:  cfg,
		snapshot: BuildSnapshot(cfg),
	}, nil
}

func (s *Service) Config() (*FullConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return CloneFullConfig(s.current)
}

func (s *Service) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *Service) Replace(cfg *FullConfig) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.applyLocked(cfg)
}

func (s *Service) AddBackend(backend BackendConfig) error {
	if backend.URL == "" {
		return errors.New("backend url cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for _, b := range next.Backends {
			if b != nil && b.URL == backend.URL {
				return fmt.Errorf("backend already exists: %s", backend.URL)
			}
		}

		cp := backend
		next.Backends = append(next.Backends, &cp)
		return nil
	})
}

func (s *Service) UpdateBackend(url string, weight int32, enabled bool) error {
	if url == "" {
		return errors.New("backend url cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for _, b := range next.Backends {
			if b != nil && b.URL == url {
				b.Weight = weight
				b.Enabled = enabled
				return nil
			}
		}
		return fmt.Errorf("backend does not exist in config: %s", url)
	})
}

func (s *Service) RemoveBackend(url string) error {
	if url == "" {
		return errors.New("backend url cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for idx, b := range next.Backends {
			if b != nil && b.URL == url {
				next.Backends = append(next.Backends[:idx], next.Backends[idx+1:]...)

				for i := range next.VirtualHosts {
					vh := &next.VirtualHosts[i]
					vh.Backends = removeString(vh.Backends, url)
					for j := range vh.PathRoutes {
						vh.PathRoutes[j].Backends = removeString(vh.PathRoutes[j].Backends, url)
					}
				}

				return nil
			}
		}

		return fmt.Errorf("backend does not exist in config: %s", url)
	})
}

func (s *Service) AddVirtualHost(vhost VirtualHost) error {
	if vhost.Domain == "" {
		return errors.New("virtual host domain cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for _, existing := range next.VirtualHosts {
			if existing.Domain == vhost.Domain {
				return fmt.Errorf("virtual host already exists: %s", vhost.Domain)
			}
		}

		if vhost.Security == nil {
			vhost.Security = defaultVirtualHostSecurity()
		}

		next.VirtualHosts = append(next.VirtualHosts, vhost)
		return nil
	})
}

func (s *Service) UpdateVirtualHost(domain string, vhost VirtualHost) error {
	if domain == "" {
		return errors.New("virtual host domain cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for i := range next.VirtualHosts {
			if next.VirtualHosts[i].Domain == domain {
				if vhost.Security == nil {
					vhost.Security = defaultVirtualHostSecurity()
				}
				vhost.Domain = domain
				next.VirtualHosts[i] = vhost
				return nil
			}
		}
		return fmt.Errorf("virtual host does not exist in config: %s", domain)
	})
}

func (s *Service) RemoveVirtualHost(domain string) error {
	if domain == "" {
		return errors.New("virtual host domain cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for i, vhost := range next.VirtualHosts {
			if vhost.Domain == domain {
				next.VirtualHosts = append(next.VirtualHosts[:i], next.VirtualHosts[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("virtual host does not exist in config: %s", domain)
	})
}

func (s *Service) UpdateGlobal(proxyPort int, strategy string) error {
	return s.update(func(next *FullConfig) error {
		if proxyPort != 0 {
			next.ProxyPort = proxyPort
		}
		if strategy != "" {
			next.LBStrategy = strategy
		}
		return nil
	})
}

func (s *Service) UpdateVirtualHostRateLimiting(domain string, rl RateLimitingConfig) error {
	return s.update(func(next *FullConfig) error {
		for i := range next.VirtualHosts {
			if next.VirtualHosts[i].Domain == domain {
				ensureSecurity(&next.VirtualHosts[i])
				next.VirtualHosts[i].Security.RateLimiting = rl
				return nil
			}
		}
		return fmt.Errorf("virtual host does not exist in config: %s", domain)
	})
}

func (s *Service) update(mutator func(next *FullConfig) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next, err := CloneFullConfig(s.current)
	if err != nil {
		return err
	}

	if err := mutator(next); err != nil {
		return err
	}

	return s.applyLocked(next)
}

func (s *Service) applyLocked(next *FullConfig) error {
	ApplyDefaults(next)

	if err := ValidateConfig(next); err != nil {
		return err
	}
	if err := SaveConfig(s.path, next); err != nil {
		return err
	}

	s.current = next
	s.snapshot = BuildSnapshot(next)
	return nil
}

func defaultVirtualHostSecurity() *SecurityConfig {
	return &SecurityConfig{
		RateLimiting: RateLimitingConfig{},
	}
}

func ensureSecurity(vhost *VirtualHost) {
	if vhost.Security == nil {
		vhost.Security = defaultVirtualHostSecurity()
	}
}

func removeString(items []string, target string) []string {
	result := items[:0]
	for _, item := range items {
		if item != target {
			result = append(result, item)
		}
	}
	return result
}
