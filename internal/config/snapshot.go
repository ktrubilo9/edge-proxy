package config

type Snapshot struct {
	Raw            *FullConfig
	BackendsByURL  map[string]*BackendConfig
	VHostsByDomain map[string]*VirtualHost
}

func BuildSnapshot(cfg *FullConfig) *Snapshot {
	if cfg == nil {
		return nil
	}

	snapshot := &Snapshot{
		Raw:            cfg,
		BackendsByURL:  make(map[string]*BackendConfig, len(cfg.Backends)),
		VHostsByDomain: make(map[string]*VirtualHost, len(cfg.VirtualHosts)),
	}

	for _, backend := range cfg.Backends {
		if backend == nil {
			continue
		}
		snapshot.BackendsByURL[backend.URL] = backend
	}

	for i := range cfg.VirtualHosts {
		vhost := &cfg.VirtualHosts[i]
		snapshot.VHostsByDomain[vhost.Domain] = vhost
	}

	return snapshot
}
