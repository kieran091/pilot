package pilot

import (
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"sort"
	"sync"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// uint32Slice implements sort.Interface for []uint32
type uint32Slice []uint32

func (s uint32Slice) Len() int           { return len(s) }
func (s uint32Slice) Less(i, j int) bool { return s[i] < s[j] }
func (s uint32Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// ConsistentHash implements a consistent hashing mechanism for load balancing.
type ConsistentHash struct {
	// replicas specifies the number of virtual nodes per physical node
	replicas int
	// keys stores the sorted hash values of all virtual nodes
	keys uint32Slice
	// hashMap maps hash values to their corresponding node identifiers
	hashMap map[uint32]string
	mu      sync.RWMutex
}

func NewConsistentHash(replicas int) *ConsistentHash {
	return &ConsistentHash{
		replicas: replicas,
		hashMap:  make(map[uint32]string),
	}
}

// hashKey computes the hash of a given key using CRC32.
func (c *ConsistentHash) hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// add adds a new node to the consistent hash ring.
func (c *ConsistentHash) add(hashNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.replicas {
		vNode := fmt.Sprintf("%s#%d", hashNode, i)
		h := c.hashKey(vNode)
		c.keys = append(c.keys, h)
		c.hashMap[h] = hashNode
	}

	sort.Sort(c.keys)
}

// remove removes a node from the consistent hash ring.
func (c *ConsistentHash) remove(hashNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.replicas {
		vNode := fmt.Sprintf("%s#%d", hashNode, i)
		h := c.hashKey(vNode)
		delete(c.hashMap, h)
	}

	newKeys := make(uint32Slice, 0, len(c.keys))
	for _, k := range c.keys {
		if _, ok := c.hashMap[k]; ok {
			newKeys = append(newKeys, k)
		}
	}

	c.keys = newKeys
	sort.Sort(c.keys)
}

func (c *ConsistentHash) get(key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.keys) == 0 {
		return "", errors.New("no nodes in hash ring")
	}

	h := c.hashKey(key)
	idx := sort.Search(len(c.keys), func(i int) bool {
		return c.keys[i] >= h
	})
	if idx == len(c.keys) {
		idx = 0
	}

	hashNode, exists := c.hashMap[c.keys[idx]]
	if !exists {
		return "", errors.Errorf("hash map entry not found for key %d", c.keys[idx])
	}

	return hashNode, nil
}

// ServiceRegistry maps service names to their corresponding endpoints
type ServiceRegistry map[string]*ServiceEndpoint

// ServiceEndpoint manages multiple instances of a service with load balancing
type ServiceEndpoint struct {

	// service is the name of the gRPC service
	service string

	// invokers maps instance identifiers to their gRPC invokers
	invokers map[string]*GRPCInvoker

	// instances holds all available service instances
	instances []*ServiceInstance

	// replicas specifies the number of virtual nodes for consistent hashing
	replicas int

	// hashRing provides consistent hashing for load balancing across instances
	hashRing *ConsistentHash

	// fds contains the protobuf file descriptor set for the service
	fds *descriptorpb.FileDescriptorSet

	mu sync.RWMutex
}

func newServiceEndpoint(replicas int, service, pb string) (*ServiceEndpoint, error) {
	pbDecode, err := base64.StdEncoding.DecodeString(pb)
	if err != nil {
		defaultLogger.Error().Err(err).Msg("failed to decode base64 proto descriptors")
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err = proto.Unmarshal(pbDecode, &fds); err != nil {
		defaultLogger.Error().Err(err).Msg("failed to unmarshal proto descriptors")
		return nil, err
	}

	return &ServiceEndpoint{
		service:   service,
		invokers:  make(map[string]*GRPCInvoker),
		instances: make([]*ServiceInstance, 0),
		replicas:  replicas,
		hashRing:  NewConsistentHash(replicas),
		fds:       &fds,
	}, nil
}

// addInstance updates the service instances and rebuilds the consistent hash ring.
func (se *ServiceEndpoint) addInstance(instance *ServiceInstance) {
	if instance == nil {
		return
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	if se.instances == nil {
		se.instances = make([]*ServiceInstance, 0)
	}
	if se.hashRing == nil {
		se.hashRing = NewConsistentHash(se.replicas)
	}
	if se.invokers == nil {
		se.invokers = make(map[string]*GRPCInvoker)
	}

	instanceExists := false
	var existingInstance *ServiceInstance
	for _, inst := range se.instances {
		if inst.Id == instance.Id {
			instanceExists = true
			existingInstance = inst
			break
		}
	}

	addressChanged := instanceExists && existingInstance.Addr != instance.Addr

	if !instanceExists {
		se.instances = append(se.instances, instance)
		hashNode := fmt.Sprintf("%s:%s", se.service, instance.Id)
		se.hashRing.add(hashNode)

		invoker, err := NewGRPCInvoker(instance.Addr, se.fds)
		if err != nil {
			return
		}
		se.invokers[hashNode] = invoker
	} else if addressChanged {
		for i, inst := range se.instances {
			if inst.Id == instance.Id {
				se.instances[i] = instance
				break
			}
		}

		ring := NewConsistentHash(se.replicas)
		for _, inst := range se.instances {
			hashNode := fmt.Sprintf("%s:%s", se.service, inst.Id)
			ring.add(hashNode)
		}
		se.hashRing = ring

		oldHashNode := fmt.Sprintf("%s:%s", se.service, existingInstance.Id)
		if invoker, exists := se.invokers[oldHashNode]; exists {
			_ = invoker.Close()
			delete(se.invokers, oldHashNode)
		}

		newHashNode := fmt.Sprintf("%s:%s", instance.Id, instance.Addr)
		invoker, err := NewGRPCInvoker(instance.Addr, se.fds)
		if err != nil {
			return
		}
		se.invokers[newHashNode] = invoker
	}
}

// removeInstance removes a service instance and updates the consistent hash ring.
func (se *ServiceEndpoint) removeInstance(instance *ServiceInstance) {
	if instance == nil {
		return
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	index := -1
	for i, inst := range se.instances {
		if inst.Id == instance.Id {
			index = i
			break
		}
	}

	if index != -1 {
		se.instances = append(se.instances[:index], se.instances[index+1:]...)
		hashNode := fmt.Sprintf("%s:%s", se.service, instance.Id)

		se.hashRing.remove(hashNode)

		if invoker, exists := se.invokers[hashNode]; exists {
			_ = invoker.Close()
			delete(se.invokers, hashNode)

			defaultLogger.Info().
				Str("instance_id", instance.Id).
				Msg("instance removed successfully")
		}
	} else {
		defaultLogger.Warn().
			Str("instance_id", instance.Id).
			Msg("instance not found for removal")
	}
}

func (se *ServiceEndpoint) len() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return len(se.instances)
}

// GetInvoker returns an InvokerFunc that uses consistent hashing to select the appropriate service instance.
func (se *ServiceEndpoint) GetInvoker(service, method string) InvokerFunc {
	return func(c *Context, key string, input []byte) ([]byte, error) {
		se.mu.RLock()
		defer se.mu.RUnlock()

		if se.hashRing == nil {
			return nil, errors.New("hash ring not initialized")
		}

		if len(se.instances) == 0 {
			return nil, errors.New("no available instances")
		}

		hashNode, err := se.hashRing.get(key)
		if err != nil {
			return nil, err
		}

		invoker, exists := se.invokers[hashNode]
		if !exists {
			return nil, errors.Errorf("invoker not found for instance %s", hashNode)
		}

		return invoker.Invoke(c.ctx, service, method, input)
	}
}

func (se *ServiceEndpoint) Close() {
	se.mu.Lock()
	defer se.mu.Unlock()

	for _, invoker := range se.invokers {
		_ = invoker.Close()
	}

	se.invokers = make(map[string]*GRPCInvoker)
	se.instances = nil
	se.hashRing = nil
}
