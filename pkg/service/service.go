package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"traefik-lazyload/pkg/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

type Core struct {
	mux  sync.Mutex
	term chan bool

	client *client.Client

	active map[string]*ContainerState // cid -> state
}

func New(client *client.Client, pollRate time.Duration) (*Core, error) {
	// Test client and report
	if info, err := client.Info(context.Background()); err != nil {
		return nil, err
	} else {
		logrus.Infof("Connected docker to %s (v%s)", info.Name, info.ServerVersion)
	}

	// Make core
	ret := &Core{
		client: client,
		active: make(map[string]*ContainerState),
		term:   make(chan bool),
	}

	ret.Poll() // initial force-poll to update
	go ret.pollThread(pollRate)

	return ret, nil
}

func (s *Core) Close() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.term <- true
	return s.client.Close()
}

func (s *Core) StartHost(hostname string) (*ContainerState, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx := context.Background()

	ct, err := s.findContainerByHostname(ctx, hostname)
	if err != nil {
		logrus.Warnf("Unable to find container for host %s: %s", hostname, err)
		return nil, err
	}

	if ets, exists := s.active[ct.ID]; exists {
		logrus.Debugf("Asked to start host, but we already think it's started: %s", ets.name)
		return ets, nil
	}

	// add to active pool
	logrus.Infof("Starting container for %s...", hostname)
	ets := newStateFromContainer(ct)
	s.active[ct.ID] = ets
	ets.pinned = true // pin while starting

	go func() {
		defer func() {
			s.mux.Lock()
			ets.pinned = false
			ets.lastActivity = time.Now()
			s.mux.Unlock()
		}()
		s.startDependencyFor(ctx, ets.needs, containerShort(ct))
		s.startContainerSync(ctx, ct)
	}()

	return ets, nil
}

// Stop all running containers pined with the configured label
func (s *Core) StopAll() {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx := context.Background()

	logrus.Info("Stopping all containers...")
	for cid, ct := range s.active {
		logrus.Infof("Stopping %s...", ct.name)
		s.client.ContainerStop(ctx, cid, container.StopOptions{})
		delete(s.active, cid)
	}
}

func (s *Core) startContainerSync(ctx context.Context, ct *types.Container) error {
	if isRunning(ct) {
		return nil
	}

	s.mux.Lock()
	defer s.mux.Unlock()

	if err := s.client.ContainerStart(ctx, ct.ID, types.ContainerStartOptions{}); err != nil {
		logrus.Warnf("Error starting container %s: %s", containerShort(ct), err)
		return err
	} else {
		logrus.Infof("Started container %s", containerShort(ct))
	}
	return nil
}

func (s *Core) startDependencyFor(ctx context.Context, needs []string, forContainer string) {
	for _, dep := range needs {
		providers, err := s.findContainersByDepProvider(ctx, dep)

		if err != nil {
			logrus.Errorf("Error finding dependency provider for %s: %v", dep, err)
		} else if len(providers) == 0 {
			logrus.Warnf("Unable to find any container that provides %s for %s", dep, forContainer)
		} else {
			for _, provider := range providers {
				if !isRunning(&provider) {
					logrus.Infof("Starting dependency for %s: %s", forContainer, containerShort(&provider))

					s.startContainerSync(ctx, &provider)

					delay, _ := labelOrDefaultDuration(&provider, "provides.delay", 2*time.Second)
					logrus.Debugf("Delaying %s to start %s", delay.String(), dep)
					time.Sleep(delay)
				}
			}
		}
	}
}

func (s *Core) stopDependenciesFor(ctx context.Context, cid string, cts *ContainerState) {
	// Look at our needs, and see if anything else needs them; if not, shut down

	deps := make(map[string]bool) // dep -> needed
	for _, dep := range cts.needs {
		deps[dep] = false
	}

	for activeId, active := range s.active {
		if activeId != cid { // ignore self
			for _, need := range active.needs {
				deps[need] = true
			}
		}
	}

	for dep, needed := range deps {
		if !needed {
			logrus.Infof("Stopping dependency %s...", dep)
			containers, err := s.findContainersByDepProvider(ctx, dep)
			if err != nil {
				logrus.Errorf("Unable to find dependency provider containers for %s: %v", dep, err)
			} else if len(containers) == 0 {
				logrus.Warnf("Unable to find any containers for dependency %s", dep)
			} else {
				for _, ct := range containers {
					if isRunning(&ct) {
						logrus.Infof("Stopping %s...", containerShort(&ct))
						go s.client.ContainerStop(ctx, ct.ID, container.StopOptions{})
					}
				}
			}
		}
	}

}

// Ticker loop that will check internal state against docker state (Call Poll)
func (s *Core) pollThread(rate time.Duration) {
	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	for {
		select {
		case <-s.term:
			return
		case <-ticker.C:
			s.Poll()
		}
	}
}

// Initiate a thread-safe state-update, adding containers to the system, or
// stopping idle containers
// Will normally happen in the background with the pollThread
func (s *Core) Poll() {
	s.mux.Lock()
	defer s.mux.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.checkForNewContainersSync(ctx)
	s.watchForInactivitySync(ctx)
}

func (s *Core) checkForNewContainersSync(ctx context.Context) {
	containers, err := s.findAllLazyloadContainers(ctx, false)
	if err != nil {
		logrus.Warnf("Error checking for new containers: %v", err)
		return
	}

	runningContainers := make(map[string]*types.Container)
	for i, ct := range containers {
		if isRunning(&ct) {
			runningContainers[ct.ID] = &containers[i]
		}
	}

	// check for containers we think are running, but aren't (destroyed, error'd, stop'd via another process, etc)
	for cid, cts := range s.active {
		if _, ok := runningContainers[cid]; !ok && !cts.pinned {
			logrus.Infof("Discover container had stopped, removing %s", cts.name)
			delete(s.active, cid)
			s.stopDependenciesFor(ctx, cid, cts)
		}
	}

	// now, look for containers that are running, but aren't in our active inventory
	for _, ct := range runningContainers {
		if _, ok := s.active[ct.ID]; !ok {
			logrus.Infof("Discovered running container %s", containerShort(ct))
			s.active[ct.ID] = newStateFromContainer(ct)
		}
	}
}

func (s *Core) watchForInactivitySync(ctx context.Context) {
	for cid, cts := range s.active {
		shouldStop, err := s.checkContainerForInactivity(ctx, cid, cts)
		if err != nil {
			logrus.Warnf("error checking container state for %s: %s", cts.name, err)
		}
		if shouldStop {
			s.stopContainerAndDependencies(ctx, cid, cts)
		}
	}
}

func (s *Core) stopContainerAndDependencies(ctx context.Context, cid string, cts *ContainerState) {
	// First, stop the host container
	if err := s.client.ContainerStop(ctx, cid, container.StopOptions{}); err != nil {
		logrus.Errorf("Error stopping container %s: %s", cts.name, err)
	} else {
		logrus.Infof("Stopped container %s", cts.name)
		delete(s.active, cid)
		s.stopDependenciesFor(ctx, cid, cts)
	}
}

func (s *Core) checkContainerForInactivity(ctx context.Context, cid string, ct *ContainerState) (shouldStop bool, retErr error) {
	if ct.pinned {
		return false, nil
	}

	statsStream, err := s.client.ContainerStatsOneShot(ctx, cid)
	if err != nil {
		return false, err
	}

	var stats types.StatsJSON
	if err := json.NewDecoder(statsStream.Body).Decode(&stats); err != nil {
		return false, err
	}

	if stats.PidsStats.Current == 0 {
		// Probably stopped. Will let next poll update container
		return true, errors.New("container not running")
	}

	// check for network activity
	rx, tx := sumNetworkBytes(stats.Networks)
	if rx > ct.lastRecv || tx > ct.lastSend {
		ct.lastRecv = rx
		ct.lastSend = tx
		ct.lastActivity = time.Now()
		return false, nil
	}

	// No activity, stop?
	if time.Now().After(ct.lastActivity.Add(ct.stopDelay)) {
		logrus.Infof("Found idle container %s...", ct.name)
		return true, nil
	}

	return false, nil
}

func (s *Core) findContainersByDepProvider(ctx context.Context, name string) ([]types.Container, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.SubLabel("provides")+"="+name)
	return s.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters,
		All:     true,
	})
}

func (s *Core) findContainerByHostname(ctx context.Context, hostname string) (*types.Container, error) {
	containers, err := s.findAllLazyloadContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if hostStr, ok := labelOrDefault(&c, "hosts", ""); ok {
			hosts := strings.Split(hostStr, ",")
			if strSliceContains(hosts, hostname) {
				return &c, nil
			}
		} else {
			// If not defined explicitely, infer from traefik route
			for k, v := range c.Labels {
				if strings.Contains(k, "traefik.http.routers.") && strings.Contains(v, hostname) { // TODO: More complex
					return &c, nil
				}
			}
		}
	}

	return nil, ErrNotFound
}

// Finds all containers on node that are labeled with lazyloader config
func (s *Core) findAllLazyloadContainers(ctx context.Context, includeStopped bool) ([]types.Container, error) {
	filters := filters.NewArgs()
	filters.Add("label", config.Model.LabelPrefix)

	return s.client.ContainerList(ctx, types.ContainerListOptions{
		All:     includeStopped,
		Filters: filters,
	})
}

// Returns all actively managed containers
func (s *Core) ActiveContainers() []*ContainerState {
	s.mux.Lock()
	defer s.mux.Unlock()

	ret := make([]*ContainerState, 0, len(s.active))
	for _, item := range s.active {
		ret = append(ret, item)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].name < ret[j].name
	})
	return ret
}

// Return all containers that qualify to be load-managed (eg. have the tag)
func (s *Core) QualifyingContainers(ctx context.Context) []ContainerWrapper {
	ct, err := s.findAllLazyloadContainers(ctx, true)
	if err != nil {
		return nil
	}

	return wrapContainers(ct...)
}

func (s *Core) ProviderContainers(ctx context.Context) []ContainerWrapper {
	filters := filters.NewArgs()
	filters.Add("label", config.SubLabel("provides"))

	ct, err := s.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters,
		All:     true,
	})
	if err != nil {
		return nil
	}

	return wrapContainers(ct...)
}
