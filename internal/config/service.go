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

func (s *Service) GetBackend(id string) *BackendConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, b := range s.current.Backends {
		if b != nil && b.Id == id {
			cp := *b
			return &cp
		}
	}
	return nil
}

func (s *Service) AddBackend(backend BackendConfig) error {
	if backend.Id == "" {
		return errors.New("backend id cannot be empty")
	}
	if backend.URL == "" {
		return errors.New("backend url cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for _, b := range next.Backends {
			if b == nil {
				continue
			}
			if b.Id == backend.Id {
				return fmt.Errorf("backend already exists: %s", backend.Id)
			}
			if b.URL == backend.URL {
				return fmt.Errorf("backend url already exists: %s", backend.URL)
			}
		}

		cp := backend
		next.Backends = append(next.Backends, &cp)
		return nil
	})
}

func (s *Service) UpdateBackend(id string, url string, weight int32, enabled bool) error {
	if id == "" {
		return errors.New("backend id cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for _, b := range next.Backends {
			if b != nil && b.Id == id {
				if url != "" {
					b.URL = url
				}
				b.Weight = weight
				b.Enabled = enabled
				return nil
			}
		}
		return fmt.Errorf("backend does not exist in config: %s", id)
	})
}

func (s *Service) RemoveBackend(id string) error {
	if id == "" {
		return errors.New("backend id cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for idx, b := range next.Backends {
			if b != nil && b.Id == id {
				next.Backends = append(next.Backends[:idx], next.Backends[idx+1:]...)

				for i := range next.VirtualHosts {
					vh := &next.VirtualHosts[i]
					vh.BackendIDs = removeString(vh.BackendIDs, id)
					for j := range vh.PathRoutes {
						vh.PathRoutes[j].BackendIDs = removeString(vh.PathRoutes[j].BackendIDs, id)
					}
				}

				return nil
			}
		}

		return fmt.Errorf("backend does not exist in config: %s", id)
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

		if vhost.SecurityPolicyID == "" {
			vhost.SecurityPolicyID = "default"
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
				if vhost.SecurityPolicyID == "" {
					vhost.SecurityPolicyID = next.VirtualHosts[i].SecurityPolicyID
				}
				if vhost.SecurityPolicyID == "" {
					vhost.SecurityPolicyID = "default"
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

func (s *Service) UpdateServer(proxyPort, adminGrpcPort int) error {
	return s.update(func(next *FullConfig) error {
		if proxyPort != 0 {
			next.Server.ProxyPort = proxyPort
		}
		if adminGrpcPort != 0 {
			next.Server.AdminGrpcPort = adminGrpcPort
		}
		return nil
	})
}

func (s *Service) SetVirtualHostSecurityPolicy(domain string, policyID string) error {
	if domain == "" {
		return errors.New("virtual host domain cannot be empty")
	}
	if policyID == "" {
		return errors.New("security policy id cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for i := range next.VirtualHosts {
			if next.VirtualHosts[i].Domain == domain {
				next.VirtualHosts[i].SecurityPolicyID = policyID
				return nil
			}
		}
		return fmt.Errorf("virtual host does not exist in config: %s", domain)
	})
}

func (s *Service) UpsertPolicy(policy SecurityPolicy) error {
	if policy.Id == "" {
		return errors.New("security policy id cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		for i := range next.Security.Policies {
			if next.Security.Policies[i].Id == policy.Id {
				next.Security.Policies[i] = policy
				return nil
			}
		}

		next.Security.Policies = append(next.Security.Policies, policy)
		return nil
	})
}

func (s *Service) UpdateLoadBalancer(strategy string) error {
	if strategy == "" {
		return errors.New("load balancer strategy cannot be empty")
	}

	return s.update(func(next *FullConfig) error {
		next.LoadBalancer.Strategy = strategy
		return nil
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

func removeString(items []string, target string) []string {
	result := items[:0]
	for _, item := range items {
		if item != target {
			result = append(result, item)
		}
	}
	return result
}
