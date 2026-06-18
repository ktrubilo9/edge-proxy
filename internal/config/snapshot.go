package config

type Snapshot struct {
	Raw            *FullConfig
	BackendsById   map[string]*BackendConfig
	PoliciesById   map[string]*SecurityPolicy
	VHostsByDomain map[string]*VirtualHost
}

func BuildSnapshot(cfg *FullConfig) *Snapshot {
	if cfg == nil {
		return nil
	}

	snapshot := &Snapshot{
		Raw:            cfg,
		BackendsById:   make(map[string]*BackendConfig, len(cfg.Backends)),
		PoliciesById:   make(map[string]*SecurityPolicy, len(cfg.Security.Policies)),
		VHostsByDomain: make(map[string]*VirtualHost, len(cfg.VirtualHosts)),
	}

	for _, backend := range cfg.Backends {
		if backend == nil {
			continue
		}
		snapshot.BackendsById[backend.Id] = backend
	}

	for i := range cfg.Security.Policies {
		policy := &cfg.Security.Policies[i]
		snapshot.PoliciesById[policy.Id] = policy
	}

	for i := range cfg.VirtualHosts {
		vhost := &cfg.VirtualHosts[i]
		snapshot.VHostsByDomain[vhost.Domain] = vhost
	}

	return snapshot
}
