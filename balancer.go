package main

func (s *Service) roundRobin() *Backend {
	s.Lock()
	defer s.Unlock()

	count := len(s.Backends)
	if count == 0 {
		return nil
	}

	// RR is always weighted.
	// we don't reduce the weight, we just distrubute exactly "Weight" calls in
	// a row
	s.lastCount++
	if s.lastCount <= int(s.Backends[s.lastBackend].Weight) {
		return s.Backends[s.lastBackend]
	}

	s.lastBackend = (s.lastBackend + 1) % count
	s.lastCount = 0
	return s.Backends[s.lastBackend]
}

func (s *Service) leastConn() *Backend {
	s.Lock()
	defer s.Unlock()

	count := uint64(len(s.Backends))
	if count == 0 {
		return nil
	}

	// return the backend with the least connections, favoring the newer backends.
	least := int64(65536)
	var backend *Backend
	for i, b := range s.Backends {
		if b.Active <= least {
			least = b.Active
			backend = b
			// keep track of this just in case
			s.lastBackend = i
		}
	}

	return backend
}
